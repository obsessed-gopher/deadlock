package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Bodies as steamcommunity.com returns them (trimmed).
const (
	steamProfileXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><profile>
	<steamID64>76561199477771562</steamID64>
	<steamID><![CDATA[someone]]></steamID>
</profile>`
	steamNotFoundXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><response><error><![CDATA[The specified profile could not be found.]]></error></response>`
)

func TestResolveVanity(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		switch r.URL.Path {
		case "/id/known/":
			_, _ = w.Write([]byte(steamProfileXML))
		case "/id/down/":
			http.Error(w, "busy", http.StatusServiceUnavailable)
		case "/id/garbage/":
			_, _ = w.Write([]byte("<html>"))
		default:
			_, _ = w.Write([]byte(steamNotFoundXML))
		}
	}))
	defer srv.Close()
	c := &SteamCommunity{BaseURL: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()

	id, err := c.ResolveVanity(ctx, "known")
	if err != nil || id != 1517505834 {
		t.Fatalf("known = %d, %v; want 1517505834", id, err)
	}
	if gotPath != "/id/known/" || gotQuery != "xml=1" {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
	if _, err := c.ResolveVanity(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown name: err = %v, want ErrNotFound", err)
	}
	for _, name := range []string{"down", "garbage"} {
		if _, err := c.ResolveVanity(ctx, name); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("%s: err = %v, want a non-NotFound error", name, err)
		}
	}
	gotPath = ""
	for _, bad := range []string{"", "a", "../profiles/1", "a b", "имя"} {
		if _, err := c.ResolveVanity(ctx, bad); !errors.Is(err, ErrBadVanity) {
			t.Errorf("%q: err = %v, want ErrBadVanity", bad, err)
		}
	}
	if gotPath != "" {
		t.Errorf("bad names reached the network: %s", gotPath)
	}
}
