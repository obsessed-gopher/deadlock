package db

import "testing"

func TestSoulsAt(t *testing.T) {
	pts := []soulPoint{{180, 1800}, {360, 3600}, {540, 3000}, {600, 6000}}
	for _, c := range []struct{ t, want uint32 }{
		{0, 0},
		{90, 900},   // from the implicit (0, 0) start
		{180, 1800}, // exactly on a snapshot
		{270, 2700}, // between snapshots
		{450, 3300}, // net worth dropped: interpolates downwards
		{600, 6000}, // last snapshot
		{601, 0},    // the match was over
		{1200, 0},
	} {
		if got := soulsAt(pts, c.t); got != c.want {
			t.Errorf("soulsAt(%d) = %d, want %d", c.t, got, c.want)
		}
	}
	if got := soulsAt(nil, 300); got != 0 {
		t.Errorf("no snapshots: %d", got)
	}
}

func TestTimeToSouls(t *testing.T) {
	pts := []soulPoint{{180, 2000}, {360, 4000}, {540, 3000}, {720, 9000}}
	for _, c := range []struct{ target, want uint32 }{
		{1000, 90},  // before the first snapshot
		{2000, 180}, // exactly
		{3000, 270}, // first crossing, not the later dip back to 3000
		{6000, 630}, // after the dip
		{9000, 720},
		{9001, 0}, // never reached
	} {
		if got := timeToSouls(pts, c.target); got != c.want {
			t.Errorf("timeToSouls(%d) = %d, want %d", c.target, got, c.want)
		}
	}
}

func TestLobbyBadge(t *testing.T) {
	for _, c := range []struct {
		b0, b1 int16
		want   uint32
	}{
		{0, 0, 0},
		{72, 0, 72},
		{0, 81, 81},
		{72, 81, 75}, // Archon 2 and Oracle 1 are 5 steps apart: Archon 4.5 -> 5
		{76, 81, 81}, // Archon 6 is next to Oracle 1: half up
		{71, 72, 72},
		{116, 116, 116},
		{70, 79, 0}, // no subtier 0 or 9
		{-1, 55, 55},
	} {
		if got := lobbyBadge(c.b0, c.b1); got != c.want {
			t.Errorf("lobbyBadge(%d, %d) = %d, want %d", c.b0, c.b1, got, c.want)
		}
	}
}
