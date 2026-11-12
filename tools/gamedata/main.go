// Command gamedata extracts heroes/items/abilities from the local Deadlock
// install into internal/gamedata/snapshot/gamedata.json (embedded into the
// server). Re-run after game patches.
//
//	go run ./tools/gamedata [-game ".../Deadlock/game/citadel"] [-check file.meta]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"deadlock_ideas/internal/gamedata"
	"deadlock_ideas/internal/meta"
)

const defaultGameDir = `C:\Program Files (x86)\Steam\steamapps\common\Deadlock\game\citadel`

func main() {
	log.SetFlags(0)
	game := flag.String("game", defaultGameDir, "path to Deadlock/game/citadel")
	out := flag.String("o", "internal/gamedata/snapshot/gamedata.json", "output file")
	check := flag.String("check", "", "optional .meta file: report hero/item ids missing from the snapshot")
	flag.Parse()

	d, err := gamedata.Extract(*game)
	if err != nil {
		log.Fatal(err)
	}
	items := 0
	for _, a := range d.Abilities {
		if a.IsItem() {
			items++
		}
	}
	fmt.Printf("game %s: %d heroes, %d abilities (%d shop items)\n", d.GameVersion, len(d.Heroes), len(d.Abilities), items)

	b, err := json.MarshalIndent(d, "", " ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote", *out)

	if *check != "" {
		mi, err := meta.ReadFile(*check)
		if err != nil {
			log.Fatal(err)
		}
		var total, missing int
		for _, p := range mi.GetPlayers() {
			if d.Hero(p.GetHeroId()) == nil {
				fmt.Println("unknown hero", p.GetHeroId())
			}
			for _, it := range p.GetItems() {
				total++
				if d.Ability(it.GetItemId()) == nil {
					missing++
					fmt.Println("unknown item id", it.GetItemId())
				}
			}
		}
		fmt.Printf("check: %d/%d item events resolved\n", total-missing, total)
	}
}
