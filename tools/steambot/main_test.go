package main

import (
	"reflect"
	"testing"
)

func TestWithBot(t *testing.T) {
	cases := []struct {
		name                 string
		users, tokens        []string
		wantUsers, wantToken []string
	}{
		{"first bot", nil, nil, []string{"new"}, []string{"tn"}},
		{"another bot", []string{"a"}, []string{"ta"}, []string{"a", "new"}, []string{"ta", "tn"}},
		{"renewed token", []string{"a", "new"}, []string{"ta", "old"}, []string{"a", "new"}, []string{"ta", "tn"}},
		{"unpaired tail dropped", []string{"a", "b"}, []string{"ta"}, []string{"a", "new"}, []string{"ta", "tn"}},
	}
	for _, c := range cases {
		u, tk := withBot(c.users, c.tokens, "new", "tn")
		if !reflect.DeepEqual(u, c.wantUsers) || !reflect.DeepEqual(tk, c.wantToken) {
			t.Errorf("%s: got %v %v, want %v %v", c.name, u, tk, c.wantUsers, c.wantToken)
		}
	}
	if got := list(" a, ,b ,"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("list = %v", got)
	}
}
