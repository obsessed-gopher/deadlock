// Command protogen regenerates all Go code from .proto files.
//
//  1. Game protos (proto/deadlock, vendored from SteamDatabase/Protobufs) ->
//     internal/pb. Valve's files are dumped from binaries and use custom
//     options (boxed_type, additional_includes, ...) that stock protoc
//     rejects, so the import closure is copied to a temp dir and sanitized.
//  2. Our API (proto/api) -> gen/go (messages, gRPC, grpc-gateway) and
//     gen/openapi (OpenAPI v2 spec, used to generate frontend types).
//
// Usage (from repo root):
//
//	go run ./tools/protogen
//
// Requires protoc (PATH, $PROTOC or winget location) and plugins in
// $GOPATH/bin: protoc-gen-go, protoc-gen-go-grpc, protoc-gen-grpc-gateway,
// protoc-gen-openapiv2.
package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	gameSrcDir    = "proto/deadlock"
	gameOutDir    = "internal/pb"
	gameGoPackage = "github.com/obsessed-gopher/deadlock/internal/pb"

	apiSrcDir     = "proto/api"
	thirdPartyDir = "proto/third_party"
	apiOutDir     = "gen/go"
	openapiOutDir = "gen/openapi"
)

// Game root files; everything they import (transitively) is generated too.
var gameRoots = []string{
	"citadel_gcmessages_common.proto", // match metadata
	"demo.proto",                      // demo container
}

// Field/enum options protoc understands natively; everything else is dropped.
var keepOptions = map[string]bool{"default": true, "packed": true, "deprecated": true, "json_name": true}

var (
	importRe  = regexp.MustCompile(`^import\s+"([^"]+)";`)
	optLineRe = regexp.MustCompile(`^\s*option\s+(\([^)]*\)|additional_includes)\s*=.*;\s*$`)
	bracketRe = regexp.MustCompile(`\s*\[([^\[\]]*)\]\s*;`)
)

func main() {
	log.SetFlags(0)
	protoc, include, err := findProtoc()
	if err != nil {
		log.Fatal(err)
	}
	// Make sure protoc finds the Go plugins even if GOPATH/bin isn't on PATH.
	if gopath, err := exec.Command("go", "env", "GOPATH").Output(); err == nil {
		bin := filepath.Join(strings.TrimSpace(string(gopath)), "bin")
		os.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	genGame(protoc, include)
	genAPI(protoc, include)
}

func genGame(protoc, include string) {
	files, err := closure(gameRoots)
	if err != nil {
		log.Fatal(err)
	}
	tmp, err := os.MkdirTemp("", "protogen-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	for _, f := range files {
		if err := sanitize(filepath.Join(gameSrcDir, f), filepath.Join(tmp, f)); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
	}
	resetDir(gameOutDir, "*.pb.go")

	args := []string{"-I", tmp, "--go_out=" + gameOutDir, "--go_opt=paths=source_relative"}
	if include != "" {
		args = append(args, "-I", include)
	}
	for _, f := range files {
		args = append(args, "--go_opt=M"+f+"="+gameGoPackage)
	}
	run(protoc, append(args, files...)...)
	fmt.Printf("game: %d files -> %s\n", len(files), gameOutDir)
}

func genAPI(protoc, include string) {
	var files []string
	err := filepath.WalkDir(apiSrcDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".proto") {
			rel, _ := filepath.Rel(apiSrcDir, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	os.RemoveAll(apiOutDir)
	os.RemoveAll(openapiOutDir)
	for _, d := range []string{apiOutDir, openapiOutDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Fatal(err)
		}
	}

	args := []string{
		"-I", apiSrcDir, "-I", thirdPartyDir,
		"--go_out=" + apiOutDir, "--go_opt=paths=source_relative",
		"--go-grpc_out=" + apiOutDir, "--go-grpc_opt=paths=source_relative",
		"--grpc-gateway_out=" + apiOutDir, "--grpc-gateway_opt=paths=source_relative",
		"--openapiv2_out=" + openapiOutDir, "--openapiv2_opt=allow_merge=true,merge_file_name=api",
	}
	if include != "" {
		args = append(args, "-I", include)
	}
	run(protoc, append(args, files...)...)
	fmt.Printf("api: %d files -> %s, %s\n", len(files), apiOutDir, openapiOutDir)
}

func run(bin string, args ...string) {
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("protoc: %v", err)
	}
}

func resetDir(dir, pattern string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(dir, pattern))
	for _, f := range old {
		os.Remove(f)
	}
}

// closure returns roots plus all transitively imported local files.
func closure(roots []string) ([]string, error) {
	seen := map[string]bool{}
	var order []string
	var visit func(string) error
	visit = func(f string) error {
		if seen[f] || strings.HasPrefix(f, "google/") {
			return nil
		}
		seen[f] = true
		order = append(order, f)
		fh, err := os.Open(filepath.Join(gameSrcDir, f))
		if err != nil {
			return err
		}
		defer fh.Close()
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			if m := importRe.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
				if err := visit(m[1]); err != nil {
					return err
				}
			}
		}
		return sc.Err()
	}
	for _, r := range roots {
		if err := visit(r); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func sanitize(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	var out strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if optLineRe.MatchString(line) {
			continue
		}
		line = bracketRe.ReplaceAllStringFunc(line, func(m string) string {
			inner := bracketRe.FindStringSubmatch(m)[1]
			var kept []string
			for _, opt := range splitOptions(inner) {
				key := strings.TrimSpace(strings.SplitN(opt, "=", 2)[0])
				if keepOptions[key] {
					kept = append(kept, strings.TrimSpace(opt))
				}
			}
			if len(kept) == 0 {
				return ";"
			}
			return " [" + strings.Join(kept, ", ") + "];"
		})
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return os.WriteFile(dst, []byte(out.String()), 0o644)
}

// splitOptions splits "a = 1, b = \"x,y\"" on commas outside quotes.
func splitOptions(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		parts = append(parts, cur.String())
	}
	return parts
}

// findProtoc locates protoc and its bundled include dir (for google/protobuf/*).
func findProtoc() (bin, include string, err error) {
	bin = os.Getenv("PROTOC")
	if bin == "" {
		if p, e := exec.LookPath("protoc"); e == nil {
			bin = p
		}
	}
	if bin == "" {
		// winget installs here but PATH may not be refreshed yet.
		matches, _ := filepath.Glob(filepath.Join(os.Getenv("LOCALAPPDATA"),
			"Microsoft", "WinGet", "Packages", "Google.Protobuf_*", "bin", "protoc.exe"))
		if len(matches) > 0 {
			bin = matches[0]
		}
	}
	if bin == "" {
		return "", "", fmt.Errorf("protoc not found: install it or set PROTOC")
	}
	if real, e := filepath.EvalSymlinks(bin); e == nil {
		bin = real
	}
	inc := filepath.Join(filepath.Dir(filepath.Dir(bin)), "include")
	if _, e := os.Stat(filepath.Join(inc, "google", "protobuf", "descriptor.proto")); e == nil {
		include = inc
	}
	return bin, include, nil
}
