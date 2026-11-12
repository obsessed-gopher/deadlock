// Command server runs the gRPC API, its REST gateway and the background
// sync worker in one process.
//
// Configuration (env):
//
//	DATABASE_URL   postgres connection string (required)
//	GRPC_ADDR      gRPC listen address            (default :9090)
//	HTTP_ADDR      REST gateway listen address    (default :8080)
//	SYNC_ACCOUNTS  comma-separated account ids to re-sync periodically
//	SYNC_EVERY     re-sync period                 (default 30m)
//	META_INTERVAL  delay between metadata downloads (default 1.5s)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"

	v1 "deadlock_ideas/gen/go/deadlock/v1"
	"deadlock_ideas/internal/db"
	"deadlock_ideas/internal/gamedata"
	"deadlock_ideas/internal/server"
	"deadlock_ideas/internal/source"
	"deadlock_ideas/internal/syncer"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	grpcAddr := env("GRPC_ADDR", ":9090")
	httpAddr := env("HTTP_ADDR", ":8080")
	syncEvery, err := time.ParseDuration(env("SYNC_EVERY", "30m"))
	if err != nil {
		return fmt.Errorf("SYNC_EVERY: %w", err)
	}
	metaInterval, err := time.ParseDuration(env("META_INTERVAL", "1.5s"))
	if err != nil {
		return fmt.Errorf("META_INTERVAL: %w", err)
	}
	accounts, err := parseAccounts(os.Getenv("SYNC_ACCOUNTS"))
	if err != nil {
		return err
	}

	database, err := connectWithRetry(ctx, dbURL, log)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		return err
	}
	log.Info("database ready")

	gd, err := gamedata.Embedded()
	if err != nil {
		return err
	}
	log.Info("game data", "version", gd.GameVersion, "heroes", len(gd.Heroes), "abilities", len(gd.Abilities))

	api := source.NewDeadlockAPI()
	sy := syncer.New(database, api, api, log)
	sy.Interval = metaInterval

	// gRPC
	gs := grpc.NewServer(grpc.UnaryInterceptor(server.UnaryInterceptor(log)))
	v1.RegisterStatsServiceServer(gs, server.New(database, sy, gd))
	reflection.Register(gs)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}

	// REST gateway -> gRPC over loopback
	gwMux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions:   protojson.MarshalOptions{EmitUnpopulated: true},
		UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
	}))
	conn, err := grpc.NewClient(loopback(grpcAddr), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := v1.RegisterStatsServiceHandler(ctx, gwMux, conn); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/", gwMux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := database.Pool.Ping(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	hs := &http.Server{Addr: httpAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	errc := make(chan error, 2)
	go func() { log.Info("grpc listening", "addr", grpcAddr); errc <- gs.Serve(lis) }()
	go func() {
		log.Info("http listening", "addr", httpAddr)
		if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()
	go sy.Run(ctx)
	if len(accounts) > 0 {
		go periodicSync(ctx, sy, accounts, syncEvery, log)
	}

	select {
	case <-ctx.Done():
	case err := <-errc:
		log.Error("server stopped", "err", err)
	}
	log.Info("shutting down")
	shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hs.Shutdown(shCtx)
	gs.GracefulStop()
	return nil
}

func periodicSync(ctx context.Context, sy *syncer.Syncer, accounts []uint32, every time.Duration, log *slog.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		for _, a := range accounts {
			if _, _, err := sy.SyncPlayer(ctx, a); err != nil && ctx.Err() == nil {
				log.Warn("periodic sync", "account", a, "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// connectWithRetry waits for Postgres to come up (useful under compose).
func connectWithRetry(ctx context.Context, url string, log *slog.Logger) (*db.DB, error) {
	var lastErr error
	for i := 0; i < 30; i++ {
		d, err := db.Open(ctx, url)
		if err == nil {
			return d, nil
		}
		lastErr = err
		log.Warn("waiting for database", "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, lastErr
}

func parseAccounts(s string) ([]uint32, error) {
	var out []uint32
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("SYNC_ACCOUNTS: bad id %q", part)
		}
		if v > 1<<32 {
			out = append(out, source.SteamID64ToAccountID(v))
		} else {
			out = append(out, uint32(v))
		}
	}
	return out, nil
}

func loopback(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
