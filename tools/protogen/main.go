// Command protogen regenerates Go code for the Deadlock protobufs.
//
// Valve's .proto files (vendored from SteamDatabase/Protobufs into
// proto/deadlock) are dumped from game binaries and use custom options
// (boxed_type, additional_includes, ...) that stock protoc rejects. protogen
// copies the import closure of the root files into a temp dir, strips every
// non-standard option, and runs protoc + protoc-gen-go into internal/pb.
//
// Usage (from repo root):
//
//	go run ./tools/protogen
//
// Requires protoc and protoc-gen-go on PATH (or PROTOC env var).
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	srcDir    = "proto/deadlock"
	outDir    = "internal/pb"
	goPackage = "deadlock_ideas/internal/pb"
)

// Root files; everything they import (transitively) is generated too.
var roots = []string{
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

	files, err := closure(roots)
	if err != nil {
		log.Fatal(err)
	}

	tmp, err := os.MkdirTemp("", "protogen-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	for _, f := range files {
		if err := sanitize(filepath.Join(srcDir, f), filepath.Join(tmp, f)); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
	}

	protoc, include, err := findProtoc()
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(outDir, "*.pb.go"))
	for _, f := range old {
		os.Remove(f)
	}

	args := []string{"-I", tmp, "--go_out=" + outDir, "--go_opt=paths=source_relative"}
	if include != "" {
		args = append(args, "-I", include)
	}
	for _, f := range files {
		args = append(args, "--go_opt=M"+f+"="+goPackage)
	}
	args = append(args, files...)

	cmd := exec.Command(protoc, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("protoc: %v", err)
	}
	fmt.Printf("generated %d files into %s\n", len(files), outDir)
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
		fh, err := os.Open(filepath.Join(srcDir, f))
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
