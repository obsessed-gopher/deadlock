// Command vpkdump lists, extracts or decodes files from a VPK (dev tool).
//
//	go run ./tools/vpkdump [-grep substr] <pak01_dir.vpk>
//	go run ./tools/vpkdump -x path -o file <pak01_dir.vpk>
//	go run ./tools/vpkdump -kv path [-depth 3] [-key name] <pak01_dir.vpk>
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/obsessed-gopher/deadlock/internal/gamedata/kv3"
	"github.com/obsessed-gopher/deadlock/internal/gamedata/resource"
	"github.com/obsessed-gopher/deadlock/internal/gamedata/vpk"
)

func main() {
	grep := flag.String("grep", "", "only list paths containing this")
	x := flag.String("x", "", "extract this path")
	o := flag.String("o", "", "output file for -x")
	kv := flag.String("kv", "", "decode this compiled resource and print its KV3 DATA")
	depth := flag.Int("depth", 2, "max print depth for -kv")
	key := flag.String("key", "", "for -kv: only print this top-level key")
	flag.Parse()
	a, err := vpk.Open(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	switch {
	case *x != "":
		b, err := a.ReadFile(*x)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*o, b, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %d bytes\n", *x, len(b))
	case *kv != "":
		b, err := a.ReadFile(*kv)
		if err != nil {
			log.Fatal(err)
		}
		root, err := resource.DecodeData(b)
		if err != nil {
			log.Fatal(err)
		}
		if *key != "" {
			fmt.Printf("%s = ", *key)
			dump(root.Get(*key), 0, *depth)
			return
		}
		fmt.Printf("%d top-level keys\n", len(root.Keys))
		dump(root, 0, *depth)
	default:
		ps := a.Paths()
		sort.Strings(ps)
		n := 0
		for _, p := range ps {
			if strings.Contains(p, *grep) {
				e, _ := a.Entry(p)
				fmt.Printf("%9d  %s\n", e.Size(), p)
				n++
			}
		}
		fmt.Println("total", n, "of", len(ps))
	}
}

func dump(v any, ind, depth int) {
	pad := strings.Repeat("  ", ind)
	switch t := v.(type) {
	case *kv3.Object:
		if ind >= depth {
			fmt.Printf("{...%d keys}\n", len(t.Keys))
			return
		}
		fmt.Println("{")
		for i, k := range t.Keys {
			fmt.Printf("%s  %s = ", pad, k)
			dump(t.Values[i], ind+1, depth)
		}
		fmt.Printf("%s}\n", pad)
	case []any:
		if ind >= depth || len(t) > 16 {
			fmt.Printf("[...%d items]\n", len(t))
			return
		}
		fmt.Println("[")
		for _, e := range t {
			fmt.Printf("%s  ", pad)
			dump(e, ind+1, depth)
		}
		fmt.Printf("%s]\n", pad)
	case string:
		fmt.Printf("%q\n", t)
	case []byte:
		fmt.Printf("<blob %d bytes>\n", len(t))
	default:
		fmt.Printf("%v\n", t)
	}
}
