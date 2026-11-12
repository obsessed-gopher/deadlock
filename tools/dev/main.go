// Command dev is the project's task runner: one entry point for every check
// that must pass before a change is done. It works the same on Windows, Linux
// and in CI and needs nothing but the Go toolchain.
//
//	go run ./tools/dev <command> [args]
//
// Commands:
//
//	setup|fmt|lint|test|test-db|test-linux|gen     install pinned tools into .tools/bin and enable git hooks
//	fmt       format code (gofumpt + goimports via golangci-lint)
//	lint      run golangci-lint (extra args are passed through, e.g. --fix)
//	test      unit tests (extra args are passed to go test, e.g. -race -run X)
//	test-db   integration tests against Postgres from docker compose
//	test-linux  unit tests with the race detector in a Linux container (what CI runs)
//	gen       regenerate code from .proto files
//	vuln      scan dependencies with govulncheck
//	web       frontend checks in web/: npm ci (if needed), typecheck, tests, build
//	e2e       frontend end-to-end tests (Playwright) against the mock API
//	check     everything required before a commit: tidy, fmt, build, lint, test
//	ci        same as check, with the race detector; used by the CI workflow
//
// Tool versions are pinned in the tools table below; bump them there.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// tool is a pinned dev dependency installed with "go install" into .tools/bin.
type tool struct {
	name    string // binary name
	pkg     string // go install path
	version string
}

var (
	golangciLint = tool{"golangci-lint", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint", "v2.15.0"}
	govulncheck  = tool{"govulncheck", "golang.org/x/vuln/cmd/govulncheck", "v1.9.0"}

	// Must match the versions recorded in the headers of generated files and
	// the runtime libraries in go.mod.
	protoPlugins = []tool{
		{"protoc-gen-go", "google.golang.org/protobuf/cmd/protoc-gen-go", "v1.36.12"},
		{"protoc-gen-go-grpc", "google.golang.org/grpc/cmd/protoc-gen-go-grpc", "v1.6.2"},
		{"protoc-gen-grpc-gateway", "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway", "v2.33.0"},
		{"protoc-gen-openapiv2", "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2", "v2.33.0"},
	}
)

const (
	toolsDir = ".tools/bin"

	// Same toolchain as the Dockerfile build stage, on glibc so -race works.
	linuxImage = "golang:1.27"

	// Matches docker-compose.yml.
	defaultTestDB = "postgres://deadlock:deadlock@localhost:5432/deadlock?sslmode=disable" //nolint:gosec // local docker-compose credentials, not a secret
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	if _, err := os.Stat("go.mod"); err != nil {
		fail(errors.New("run from the repository root"))
	}
	cmd, args := os.Args[1], os.Args[2:]

	commands := map[string]func([]string) error{
		"setup":      setup,
		"fmt":        format,
		"lint":       lint,
		"test":       test,
		"test-db":    testDB,
		"test-linux": testLinux,
		"gen":        gen,
		"vuln":       vuln,
		"web":        web,
		"e2e":        e2e,
		"check":      check,
		"ci":         ci,
	}
	fn, ok := commands[cmd]
	if !ok {
		usage()
	}
	if err := fn(args); err != nil {
		fail(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./tools/dev setup|fmt|lint|test|test-db|test-linux|gen|vuln|web|e2e|check|ci [args]")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "dev:", err)
	os.Exit(1)
}

func setup([]string) error {
	for _, t := range append([]tool{golangciLint, govulncheck}, protoPlugins...) {
		if _, err := ensure(t); err != nil {
			return err
		}
	}
	return run("git", "config", "core.hooksPath", ".githooks")
}

func format([]string) error {
	bin, err := ensure(golangciLint)
	if err != nil {
		return err
	}
	return run(bin, "fmt")
}

func lint(args []string) error {
	bin, err := ensure(golangciLint)
	if err != nil {
		return err
	}
	return run(bin, append([]string{"run"}, args...)...)
}

func test(args []string) error {
	return run("go", append([]string{"test", "-shuffle=on"}, append(args, "./...")...)...)
}

func testDB(args []string) error {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		if err := run("docker", "compose", "up", "-d", "--wait", "postgres"); err != nil {
			return err
		}
		if err := os.Setenv("TEST_DATABASE_URL", defaultTestDB); err != nil {
			return err
		}
	}
	// -count=1: results depend on the database, never serve them from cache.
	return run("go", append([]string{"test", "-count=1"}, append(args, "./internal/db/...")...)...)
}

// testLinux runs the unit tests the way CI does: on Linux, with the race
// detector. Windows cannot run -race without gcc and hides bugs that depend
// on the path separator or file modes.
func testLinux(args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	docker := []string{
		"run", "--rm",
		"-v", wd + ":/src", "-w", "/src",
		"-v", "deadlock-gomod:/go/pkg/mod", "-v", "deadlock-gobuild:/root/.cache/go-build",
		linuxImage,
		"go", "test", "-race", "-shuffle=on",
	}
	return run("docker", append(docker, append(args, "./...")...)...)
}

func gen([]string) error {
	for _, t := range protoPlugins {
		if _, err := ensure(t); err != nil {
			return err
		}
	}
	return run("go", "run", "./tools/protogen")
}

func vuln([]string) error {
	bin, err := ensure(govulncheck)
	if err != nil {
		return err
	}
	return run(bin, "./...")
}

func check([]string) error {
	return checkWith()
}

func ci([]string) error {
	return checkWith("-race")
}

func checkWith(testArgs ...string) error {
	bin, err := ensure(golangciLint)
	if err != nil {
		return err
	}
	steps := []struct {
		name string
		fn   func() error
	}{
		{"go mod tidy", func() error { return run("go", "mod", "tidy", "-diff") }},
		{"format", func() error { return run(bin, "fmt", "--diff") }},
		{"build", func() error { return run("go", "build", "./...") }},
		{"lint", func() error { return run(bin, "run") }},
		{"test", func() error { return test(testArgs) }},
	}
	for _, s := range steps {
		fmt.Printf("==> %s\n", s.name)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	fmt.Println("==> all checks passed")
	return nil
}

// ensure installs the tool at its pinned version unless it is already there
// and returns the path to the binary.
func ensure(t tool) (string, error) {
	dir, err := filepath.Abs(toolsDir)
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, t.name)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	stamp := filepath.Join(dir, t.name+".version")
	if have, err := os.ReadFile(stamp); err == nil && strings.TrimSpace(string(have)) == t.version {
		if _, err := os.Stat(bin); err == nil {
			return bin, nil
		}
	}
	fmt.Printf("==> installing %s %s\n", t.name, t.version)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(context.Background(), "go", "install", t.pkg+"@"+t.version) //nolint:gosec // arguments come from the pinned table above
	cmd.Env = append(os.Environ(), "GOBIN="+dir, "GOFLAGS=")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("install %s: %w", t.name, err)
	}
	if err := os.WriteFile(stamp, []byte(t.version+"\n"), 0o600); err != nil {
		return "", err
	}
	return bin, nil
}

// web runs the frontend checks. Dependencies are installed with "npm ci"
// when node_modules is missing and always in CI, so a stale local install is
// not reinstalled on every run.
func web([]string) error {
	if err := webDeps(); err != nil {
		return err
	}
	return runIn(webDir, "npm", "run", "check")
}

const webDir = "web"

func webDeps() error {
	if _, err := os.Stat(filepath.Join(webDir, "node_modules")); err == nil && os.Getenv("CI") == "" {
		return nil
	}
	return runIn(webDir, "npm", "ci", "--no-audit", "--no-fund")
}

// e2e builds the app and runs Playwright against the mock API. Extra args go
// to "playwright test" (e.g. -g "sync" or --headed).
func e2e(args []string) error {
	if err := webDeps(); err != nil {
		return err
	}
	install := []string{"playwright", "install", "chromium"}
	if os.Getenv("CI") != "" {
		install = append(install, "--with-deps")
	}
	if err := runIn(webDir, "npx", install...); err != nil {
		return err
	}
	return runIn(webDir, "npm", append([]string{"run", "e2e", "--"}, args...)...)
}

func run(bin string, args ...string) error {
	return runIn("", bin, args...)
}

func runIn(dir, bin string, args ...string) error {
	cmd := exec.CommandContext(context.Background(), bin, args...) //nolint:gosec // dev tool: runs fixed commands with pass-through args from the developer
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", filepath.Base(bin), strings.Join(args, " "), err)
	}
	return nil
}
