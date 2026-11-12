package gamedata

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/obsessed-gopher/deadlock/internal/meta"
)

func TestHashKey(t *testing.T) {
	// ids observed in real match metadata
	cases := map[string]uint32{
		"upgrade_rapid_rounds": 668299740,
		"ability_sleep_dagger": 2948410412,
		"upgrade_magic_storm":  1292979587,
		"UPGRADE_CAPACITOR":    710436191, // case-insensitive
	}
	for key, want := range cases {
		if got := HashKey(key); got != want {
			t.Errorf("HashKey(%q) = %d, want %d", key, got, want)
		}
	}
}

// The embedded snapshot must resolve every hero and item in a real match.
func TestEmbeddedResolvesMatch(t *testing.T) {
	d, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if h := d.Hero(13); h == nil || h.Key != "hero_haze" || h.Name != "Haze" {
		t.Errorf("hero 13 = %+v", h)
	}
	if a := d.Ability(668299740); a == nil || !a.IsItem() || a.Slot != "weapon" || a.Tier != 1 {
		t.Errorf("rapid rounds = %+v", a)
	}

	mi, err := meta.ReadFile("../meta/testdata/134969900.meta.zst")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range mi.GetPlayers() {
		if h := d.Hero(p.GetHeroId()); h == nil || h.Name == "" {
			t.Errorf("hero %d unresolved", p.GetHeroId())
		}
		for _, it := range p.GetItems() {
			if d.Ability(it.GetItemId()) == nil {
				t.Errorf("item %d unresolved", it.GetItemId())
			}
		}
	}
}

func TestKV1Tokens(t *testing.T) {
	in := "\ufeff\"lang\"\n{\n  // comment \"x\"\n  \"Tokens\" { \"a\" \"say \\\"hi\\\"\" [$X360]\n\"b\"\t\"c\" }\n}"
	want := []string{"lang", "{", "Tokens", "{", "a", `say "hi"`, "b", "c", "}", "}"}
	if got := kv1Tokens(in); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestLookupStripsMarkers(t *testing.T) {
	l := localization{"hero_haze:n": "#|f|#Пелена"}
	if got := l.lookup("HERO_HAZE:n"); got != "Пелена" {
		t.Errorf("got %q", got)
	}
}

// Full extraction from the local install; skipped when the game isn't there.
func TestExtractLocalInstall(t *testing.T) {
	dir := os.Getenv("DEADLOCK_GAME_DIR")
	if dir == "" {
		dir = `C:\Program Files (x86)\Steam\steamapps\common\Deadlock\game\citadel`
	}
	if _, err := os.Stat(filepath.Join(dir, "pak01_dir.vpk")); err != nil {
		t.Skip("Deadlock not installed")
	}
	d, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Heroes) < 30 || len(d.Abilities) < 300 {
		t.Errorf("got %d heroes, %d abilities", len(d.Heroes), len(d.Abilities))
	}
}
