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
//	DEADLOCK_API_KEY  optional deadlock-api key: higher rate limits
//	STEAM_BOT_USER, STEAM_BOT_TOKEN  optional Steam bot for the Deadlock GC
//	               (go run ./tools/steambot login): history and salts first-hand
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
	"syscall"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/db"
	"github.com/obsessed-gopher/deadlock/internal/gamedata"
	"github.com/obsessed-gopher/deadlock/internal/server"
	"github.com/obsessed-gopher/deadlock/internal/source"
	"github.com/obsessed-gopher/deadlock/internal/steamgc"
	"github.com/obsessed-gopher/deadlock/internal/syncer"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
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

	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}

	database, err := connectWithRetry(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		return err
	}
	// Nothing is syncing yet, so every "running" state is left over from a
	// process that died mid-sync.
	if n, err := database.ResetInterruptedSyncs(ctx); err != nil {
		return err
	} else if n > 0 {
		log.Warn("reset interrupted syncs", "players", n)
	}
	log.Info("database ready")

	gd, err := gamedata.Embedded()
	if err != nil {
		return err
	}
	log.Info("game data", "version", gd.GameVersion, "heroes", len(gd.Heroes), "abilities", len(gd.Abilities))

	api := source.NewDeadlockAPI()
	api.APIKey = cfg.DeadlockAPIKey
	log.Info("deadlock-api", "key", cfg.DeadlockAPIKey != "")
	var history source.HistoryProvider = api
	var salts source.SaltProvider = api
	if len(cfg.SteamBots) > 0 {
		// The GC is first-hand and fresh; deadlock-api has the full history
		// and covers the GC being down or out of budget.
		// Budgets and leases live in the database: a restart keeps the
		// budget, and with several processes each bot runs in one of them.
		bots := steamgc.NewPool(log, cfg.SteamBots, database)
		go bots.Run(ctx)
		history = source.MergedHistory{
			Providers: []source.HistoryProvider{bots, api},
			OnError: func(i int, err error) {
				if errors.Is(err, steamgc.ErrNotFriend) {
					return // the usual case for players the bots don't know
				}
				log.Warn("match history source failed", "source", []string{"steam gc", "deadlock-api"}[i], "err", err)
			},
		}
		salts = source.SaltChain{bots, api}
	}
	log.Info("steam gc", "bots", len(cfg.SteamBots))
	sy := syncer.New(database, history, salts, log)
	sy.Interval = cfg.MetaInterval
	sy.Profiles = api
	sy.Meta = api

	// gRPC
	gs := grpc.NewServer(grpc.UnaryInterceptor(server.UnaryInterceptor(log)))
	v1.RegisterStatsServiceServer(gs, server.New(database, sy, source.NewSteamCommunity(), gd))
	reflection.Register(gs)
	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	// REST gateway -> gRPC over loopback
	gwMux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions:   protojson.MarshalOptions{EmitUnpopulated: true},
		UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
	}))
	conn, err := grpc.NewClient(loopback(cfg.GRPCAddr), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := v1.RegisterStatsServiceHandler(ctx, gwMux, conn); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/", gwMux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := database.Ping(r.Context()); err != nil {
			log.Warn("healthz", "err", err)
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	hs := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	errc := make(chan error, 2)
	go func() {
		log.Info("grpc listening", "addr", cfg.GRPCAddr)
		if err := gs.Serve(lis); err != nil {
			errc <- fmt.Errorf("grpc: %w", err)
		}
	}()
	go func() {
		log.Info("http listening", "addr", cfg.HTTPAddr)
		if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("http: %w", err)
		}
	}()
	go sy.Run(ctx)
	go sy.RunProfiles(ctx)
	if len(cfg.SyncAccounts) > 0 {
		go periodicSync(ctx, sy, cfg.SyncAccounts, cfg.SyncEvery, log)
	}

	// A listener dying is fatal: report it so the process exits non-zero and
	// the supervisor restarts it. A signal is a clean shutdown.
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
	}
	log.Info("shutting down")
	shCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := hs.Shutdown(shCtx); err != nil {
		log.Warn("http shutdown", "err", err)
	}
	gs.GracefulStop()
	return runErr
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
	for range 30 {
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
