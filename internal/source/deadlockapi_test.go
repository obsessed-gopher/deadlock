package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A trimmed /v1/players/steam answer as deadlock-api returns it.
const profilesJSON = `[{"account_id":1517505834,"personaname":"someone","profileurl":"https://steamcommunity.com/profiles/76561199477771562/",
"avatar":"https://avatars.steamstatic.com/a.jpg","avatarmedium":"https://avatars.steamstatic.com/a_medium.jpg",
"avatarfull":"https://avatars.steamstatic.com/a_full.jpg","realname":null,"countrycode":null,"last_updated":"2026-11-12T22:09:30Z",
"matches_played_last_30d":3,"friends":[{"account_id":166573769,"friend_since":"2023-04-17T13:30:20Z"}]}]`

func TestSteamProfiles(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("account_ids")
		if gotQuery == "7" {
			http.Error(w, "No Steam profile found.", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(profilesJSON))
	}))
	defer srv.Close()
	c := &DeadlockAPI{BaseURL: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()

	ps, err := c.SteamProfiles(ctx, []uint32{1517505834, 42})
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "1517505834,42" {
		t.Errorf("account_ids = %q", gotQuery)
	}
	if len(ps) != 1 {
		t.Fatalf("profiles = %+v", ps)
	}
	p := ps[0]
	if p.AccountID != 1517505834 || p.PersonaName != "someone" || p.AvatarMedium == "" || p.CountryCode != nil ||
		len(p.Friends) != 1 || p.Friends[0].AccountID != 166573769 || p.Friends[0].FriendSince == nil {
		t.Errorf("profile = %+v", p)
	}

	if ps, err := c.SteamProfiles(ctx, []uint32{7}); err != nil || ps != nil {
		t.Errorf("none found = %v, %v; want nil, nil", ps, err)
	}
	gotQuery = ""
	if ps, err := c.SteamProfiles(ctx, nil); err != nil || ps != nil || gotQuery != "" {
		t.Errorf("no ids = %v, %v (query %q); want no request", ps, err, gotQuery)
	}
	if _, err := c.SteamProfiles(ctx, make([]uint32, MaxProfileBatch+1)); err == nil {
		t.Error("over the batch limit: want an error")
	}
}
