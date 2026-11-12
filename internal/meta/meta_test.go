package meta

import "testing"

// testdata/134969900.meta.zst is a real file from Valve's replay CDN
// (zstd despite the .bz2 name Valve serves it under).
func TestReadFile(t *testing.T) {
	mi, err := ReadFile("testdata/134969900.meta.zst")
	if err != nil {
		t.Fatal(err)
	}
	if got := mi.GetMatchId(); got != 134969900 {
		t.Errorf("match_id = %d", got)
	}
	if got := mi.GetDurationS(); got != 1728 {
		t.Errorf("duration = %d, want 1728", got)
	}
	if got := len(mi.GetPlayers()); got != 12 {
		t.Fatalf("players = %d, want 12", got)
	}

	var me bool
	for _, p := range mi.GetPlayers() {
		if len(p.GetStats()) == 0 {
			t.Errorf("slot %d: no stat snapshots", p.GetPlayerSlot())
		}
		if p.GetAccountId() != 1517505834 {
			continue
		}
		me = true
		if p.GetHeroId() != 13 || p.GetKills() != 8 || p.GetDeaths() != 8 || p.GetAssists() != 8 {
			t.Errorf("player line = hero %d %d/%d/%d", p.GetHeroId(), p.GetKills(), p.GetDeaths(), p.GetAssists())
		}
		if got := len(p.GetDeathDetails()); got != 8 {
			t.Errorf("death details = %d, want 8", got)
		}
		if p.GetNetWorth() != 29130 {
			t.Errorf("net worth = %d", p.GetNetWorth())
		}
	}
	if !me {
		t.Error("account 1517505834 not in match")
	}
}

func TestDecodeGarbage(t *testing.T) {
	if _, err := Decode([]byte{0xff, 0xff, 0xff}); err == nil {
		t.Error("expected error")
	}
}
