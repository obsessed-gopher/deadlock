package steamgc

import (
	"errors"
	"testing"

	gc "github.com/paralin/go-deadlock/protocol"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

func TestSaltsErr(t *testing.T) {
	type R = gc.CMsgClientToGCGetMatchMetaDataResponse_EResult
	cases := []struct {
		r            R
		ok, gone, rl bool
	}{
		{r: gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_Success, ok: true},
		{r: gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_InvalidMatch, gone: true},
		{r: gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_RateLimited, rl: true},
		{r: gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_TooBusy, rl: true},
		{r: gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_MatchInFlight, rl: true},
		{r: gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_InvalidPermission},
		{r: gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_InternalError},
	}
	for _, c := range cases {
		err := saltsErr(c.r)
		_, rl := errors.AsType[*source.RateLimitError](err)
		if (err == nil) != c.ok || errors.Is(err, source.ErrNotFound) != c.gone || rl != c.rl {
			t.Errorf("%v: err %v; want ok=%v gone=%v rate-limited=%v", c.r, err, c.ok, c.gone, c.rl)
		}
	}
}

func TestHistoryErr(t *testing.T) {
	if err := historyErr(gc.CMsgClientToGCGetMatchHistoryResponse_k_eResult_Success); err != nil {
		t.Errorf("success: %v", err)
	}
	if _, rl := errors.AsType[*source.RateLimitError](historyErr(gc.CMsgClientToGCGetMatchHistoryResponse_k_eResult_RateLimited)); !rl {
		t.Error("rate limited: want a RateLimitError")
	}
	err := historyErr(gc.CMsgClientToGCGetMatchHistoryResponse_k_eResult_InvalidPermission)
	if _, rl := errors.AsType[*source.RateLimitError](err); err == nil || rl {
		t.Errorf("no permission (not a friend): want a plain error, got %v", err)
	}
}
