package gamedata

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/obsessed-gopher/deadlock/internal/gamedata/kv3"
	"github.com/obsessed-gopher/deadlock/internal/gamedata/resource"
	"github.com/obsessed-gopher/deadlock/internal/gamedata/vpk"
)

// Extract builds a snapshot from a Deadlock install, where gameDir is
// ".../steamapps/common/Deadlock/game/citadel".
func Extract(gameDir string) (*Data, error) {
	pak, err := vpk.Open(filepath.Join(gameDir, "pak01_dir.vpk"))
	if err != nil {
		return nil, err
	}
	en, err := loadLocalization(gameDir, "english")
	if err != nil {
		return nil, err
	}
	ru, err := loadLocalization(gameDir, "russian")
	if err != nil {
		return nil, err
	}

	d := &Data{GameVersion: readGameVersion(gameDir)}

	heroes, err := decode(pak, "scripts/heroes.vdata_c")
	if err != nil {
		return nil, err
	}
	for i, key := range heroes.Keys {
		h, ok := heroes.Values[i].(*kv3.Object)
		if !ok || h.Get("m_HeroID") == nil {
			continue // generic_data_type, hero_base, ...
		}
		d.Heroes = append(d.Heroes, &Hero{
			ID:       uint32(h.Int("m_HeroID")),
			Key:      key,
			Name:     en.lookup(key + ":n"),
			NameRU:   ru.lookup(key + ":n"),
			Type:     enumSuffix(h.Str("m_eHeroType"), "ECitadelHeroType_"),
			State:    enumSuffix(h.Str("m_eHeroDevelopmentState"), "EHeroDevState_"),
			Disabled: h.Get("m_bDisabled") == true,
			InDev:    h.Get("m_bInDevelopment") == true,
			Image:    h.Str("m_strIconImageSmall"),
		})
	}
	sort.Slice(d.Heroes, func(i, j int) bool { return d.Heroes[i].ID < d.Heroes[j].ID })

	abilities, err := decode(pak, "scripts/abilities.vdata_c")
	if err != nil {
		return nil, err
	}
	seen := map[uint32]string{}
	for i, key := range abilities.Keys {
		a, ok := abilities.Values[i].(*kv3.Object)
		if !ok {
			continue
		}
		typ := enumSuffix(a.Str("m_eAbilityType"), "EAbilityType_")
		if typ == "" {
			continue // abstract bases etc.
		}
		id := HashKey(key)
		if other, dup := seen[id]; dup {
			return nil, fmt.Errorf("gamedata: hash collision %q vs %q", key, other)
		}
		seen[id] = key
		ab := &Ability{
			ID:     id,
			Key:    key,
			Name:   en.lookup(key),
			NameRU: ru.lookup(key),
			Type:   typ,
			Image:  a.Str("m_strAbilityImage"),
		}
		if ab.IsItem() {
			ab.Slot = itemSlot(a.Str("m_eItemSlotType"))
			fmt.Sscanf(a.Str("m_iItemTier"), "EModTier_%d", &ab.Tier)
		}
		d.Abilities = append(d.Abilities, ab)
	}
	sort.Slice(d.Abilities, func(i, j int) bool { return d.Abilities[i].Key < d.Abilities[j].Key })

	d.index()
	return d, nil
}

func decode(pak *vpk.Archive, path string) (*kv3.Object, error) {
	b, err := pak.ReadFile(path)
	if err != nil {
		return nil, err
	}
	obj, err := resource.DecodeData(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return obj, nil
}

// enumSuffix turns "EAbilityType_Item" into "item".
func enumSuffix(v, prefix string) string {
	if !strings.HasPrefix(v, prefix) {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(v, prefix))
}

func itemSlot(v string) string {
	switch v {
	case "EItemSlotType_WeaponMod":
		return "weapon"
	case "EItemSlotType_Armor":
		return "vitality"
	case "EItemSlotType_Tech":
		return "spirit"
	}
	return enumSuffix(v, "EItemSlotType_")
}

func readGameVersion(gameDir string) string {
	f, err := os.Open(filepath.Join(gameDir, "steam.inf"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "ClientVersion="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// --- localization (KeyValues1 "lang" files) ---

type localization map[string]string

// Russian strings may carry grammatical markers like "#|f|#Пелена".
var markerRe = regexp.MustCompile(`^#\|[a-z]+\|#`)

func (l localization) lookup(key string) string {
	return markerRe.ReplaceAllString(l[strings.ToLower(key)], "")
}

func loadLocalization(gameDir, lang string) (localization, error) {
	files, err := filepath.Glob(filepath.Join(gameDir, "resource", "localization", "*", "*_"+lang+".txt"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("gamedata: no %s localization files", lang)
	}
	out := localization{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		toks := kv1Tokens(string(b))
		// "lang" { "Language" "x" "Tokens" { "k" "v" ... } }: collect every
		// string pair at depth 2.
		depth := 0
		for i := 0; i < len(toks); i++ {
			switch toks[i] {
			case "{":
				depth++
				continue
			case "}":
				depth--
				continue
			}
			if depth == 2 && i+1 < len(toks) && toks[i+1] != "{" && toks[i+1] != "}" {
				out[strings.ToLower(toks[i])] = toks[i+1]
				i++
			}
		}
	}
	return out, nil
}

// kv1Tokens splits KeyValues1 text into quoted strings and braces, skipping
// // comments and [$PLATFORM] conditionals. Braces are returned as "{"/"}";
// a quoted "{" is indistinguishable, which is fine for localization files.
func kv1Tokens(s string) []string {
	s = strings.TrimPrefix(s, string(rune(0xFEFF)))
	var toks []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			var b strings.Builder
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					switch s[i+1] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					default:
						b.WriteByte(s[i+1])
					}
					i += 2
					continue
				}
				b.WriteByte(s[i])
				i++
			}
			i++
			toks = append(toks, b.String())
		case c == '{' || c == '}':
			toks = append(toks, string(c))
			i++
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '[':
			for i < len(s) && s[i] != ']' {
				i++
			}
			i++
		default:
			i++
		}
	}
	return toks
}
