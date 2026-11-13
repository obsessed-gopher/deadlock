// Command steambot sets up and checks the Steam bot account that talks to the
// Deadlock Game Coordinator (dev tool, run by hand in your own terminal).
//
//	go run ./tools/steambot login            # log the bot in, save its token to .env
//	go run ./tools/steambot probe [account]  # newest matches of account + salts of the newest
//
// login asks for the bot's login and password (the password is not echoed,
// used once and not stored), walks through Steam Guard and writes
// STEAM_BOT_USER and STEAM_BOT_TOKEN to .env. probe reads them from the
// environment or .env.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/obsessed-gopher/deadlock/internal/source"
	"github.com/obsessed-gopher/deadlock/internal/steamgc"
)

const envFile = ".env"

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if len(args) == 0 {
		return errors.New("usage: steambot login | probe [account_id]")
	}
	var err error
	switch args[0] {
	case "login":
		err = login(ctx)
	case "probe":
		err = probe(ctx, args[1:])
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	return err
}

func login(ctx context.Context) error {
	in := bufio.NewReader(os.Stdin)
	user := prompt(in, "Steam login of the bot: ")
	fmt.Print("Password (not shown): ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	token, err := steamgc.Login(ctx, user, string(pw), func(_ context.Context, kind steamgc.GuardKind, _ string) (string, error) {
		switch kind {
		case steamgc.GuardAppApproval:
			fmt.Println("Approve the login in the Steam mobile app…")
			return "", nil
		case steamgc.GuardEmailLink:
			fmt.Println("Approve the login via the link in the email from Steam…")
			return "", nil
		case steamgc.GuardDeviceCode:
			return prompt(in, "Steam Guard code from the mobile app: "), nil
		default:
			return prompt(in, "Steam Guard code from the email: "), nil
		}
	})
	if err != nil {
		return err
	}
	if err := setEnv(envFile, map[string]string{"STEAM_BOT_USER": user, "STEAM_BOT_TOKEN": token}); err != nil {
		return err
	}
	fmt.Println("Done: STEAM_BOT_USER and STEAM_BOT_TOKEN are in .env. Check with: go run ./tools/steambot probe")
	return nil
}

func probe(ctx context.Context, args []string) error {
	env := readEnv(envFile)
	user, token := env("STEAM_BOT_USER"), env("STEAM_BOT_TOKEN")
	var account uint32
	if len(args) > 0 {
		id, err := source.ParseAccountID(args[0])
		if err != nil {
			return err
		}
		account = id
	} else {
		first, _, _ := strings.Cut(env("SYNC_ACCOUNTS"), ",")
		id, err := source.ParseAccountID(strings.TrimSpace(first))
		if err != nil {
			return errors.New("pass an account id or set SYNC_ACCOUNTS")
		}
		account = id
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c, err := steamgc.Dial(ctx, logger, user, token)
	if err != nil {
		return err
	}
	defer c.Close()

	ms, err := c.MatchHistory(ctx, account)
	if err != nil {
		return err
	}
	fmt.Printf("match history of %d: %d matches\n", account, len(ms))
	for i, m := range ms {
		if i == 5 {
			break
		}
		fmt.Printf("  %d  %s  hero %d  %d/%d/%d\n", m.MatchID, time.Unix(m.StartTime, 0).UTC().Format(time.DateTime), m.HeroID, m.Kills, m.Deaths, m.Assists)
	}
	if len(ms) == 0 {
		return nil
	}
	s, err := c.Salts(ctx, ms[0].MatchID)
	if err != nil {
		return err
	}
	fmt.Printf("salts of %d: cluster %d, metadata %s\n", s.MatchID, s.ClusterID, s.MetadataURL())
	return nil
}

func prompt(in *bufio.Reader, q string) string {
	fmt.Print(q)
	s, _ := in.ReadString('\n')
	return strings.TrimSpace(s)
}

// readEnv looks a key up in the process environment, then in the env file.
func readEnv(path string) func(string) string {
	file := map[string]string{}
	if b, err := os.ReadFile(path); err == nil {
		for line := range strings.Lines(string(b)) {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && !strings.HasPrefix(k, "#") {
				file[k] = strings.Trim(v, `"'`)
			}
		}
	}
	return func(k string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return file[k]
	}
}

// setEnv replaces or appends keys in the env file, keeping everything else.
func setEnv(path string, kv map[string]string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var out []string
	for line := range strings.Lines(string(b)) {
		line = strings.TrimRight(line, "\r\n")
		k, _, _ := strings.Cut(line, "=")
		if _, ok := kv[strings.TrimSpace(k)]; ok {
			continue
		}
		out = append(out, line)
	}
	for _, k := range []string{"STEAM_BOT_USER", "STEAM_BOT_TOKEN"} {
		if v, ok := kv[k]; ok {
			out = append(out, k+"="+v)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o600)
}
