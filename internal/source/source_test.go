package source

import "testing"

func TestParseAccountID(t *testing.T) {
	cases := []struct {
		in      string
		want    uint32
		wantErr bool
	}{
		{in: "1517505834", want: 1517505834},
		{in: " 42 ", want: 42},
		{in: "4294967295", want: 4294967295},
		{in: "76561199477771562", want: 1517505834}, // SteamID64
		{in: "0", wantErr: true},
		{in: "", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "-1", wantErr: true},
		{in: "4294967296", wantErr: true},        // above uint32, below the SteamID64 range
		{in: "76561197960265728", wantErr: true}, // SteamID64 of account 0
		{in: "99999999999999999999", wantErr: true},
	}
	for _, c := range cases {
		got, err := ParseAccountID(c.in)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("ParseAccountID(%q) = %d, %v; want %d, error=%v", c.in, got, err, c.want, c.wantErr)
		}
	}
}

func TestSaltsURLs(t *testing.T) {
	s := Salts{MatchID: 134969900, ClusterID: 404, MetadataSalt: 111, ReplaySalt: 222}
	if got, want := s.MetadataURL(), "http://replay404.valve.net/1422450/134969900_111.meta.bz2"; got != want {
		t.Errorf("MetadataURL = %s, want %s", got, want)
	}
	if got, want := s.DemoURL(), "http://replay404.valve.net/1422450/134969900_222.dem.bz2"; got != want {
		t.Errorf("DemoURL = %s, want %s", got, want)
	}
}

func TestMatchSummaryWon(t *testing.T) {
	if !(MatchSummary{Team: 1, Result: 1}).Won() {
		t.Error("same team and result must be a win")
	}
	if (MatchSummary{Team: 0, Result: 1}).Won() {
		t.Error("different team and result must be a loss")
	}
}
