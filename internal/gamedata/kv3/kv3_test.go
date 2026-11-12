package kv3

import "testing"

// Decoding of a real document is covered by internal/gamedata/resource, which
// owns the sample file. These tests cover the accessors and input validation.

func TestObjectAccessors(t *testing.T) {
	inner := &Object{Keys: []string{"x"}, Values: []any{"y"}}
	o := &Object{
		Keys:   []string{"s", "i", "u", "f", "o", "dup", "dup"},
		Values: []any{"str", int64(-3), uint64(7), 2.9, inner, "first", "second"},
	}

	if got := o.Str("s"); got != "str" {
		t.Errorf("Str = %q", got)
	}
	if got := o.Str("i"); got != "" {
		t.Errorf("Str of a number = %q, want empty", got)
	}
	for key, want := range map[string]int64{"i": -3, "u": 7, "f": 2, "s": 0, "missing": 0} {
		if got := o.Int(key); got != want {
			t.Errorf("Int(%q) = %d, want %d", key, got, want)
		}
	}
	if o.Obj("o") != inner || o.Obj("s") != nil || o.Obj("missing") != nil {
		t.Error("Obj must return nested objects only")
	}
	if got := o.Str("dup"); got != "second" {
		t.Errorf("duplicate key: got %q, want the later value", got)
	}
	if o.Get("missing") != nil {
		t.Error("Get of a missing key must be nil")
	}
}

func TestNilObjectIsSafe(t *testing.T) {
	var o *Object
	if o.Get("a") != nil || o.Str("a") != "" || o.Int("a") != 0 || o.Obj("a") != nil {
		t.Error("accessors on a nil object must return zero values")
	}
	// Chained lookups through a missing object are the common case in extract.go.
	if got := (&Object{}).Obj("missing").Obj("deeper").Str("x"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestDecodeRejectsBadInput(t *testing.T) {
	cases := map[string][]byte{
		"empty":            nil,
		"short":            {0x04, '3'},
		"wrong magic":      {'P', 'K', 3, 4, 0, 0, 0, 0},
		"old version":      {0x03, '3', 'V', 'K', 0, 0, 0, 0},
		"future version":   {0x06, '3', 'V', 'K', 0, 0, 0, 0},
		"truncated header": {0x04, '3', 'V', 'K', 1, 2, 3},
	}
	for name, in := range cases {
		if _, err := Decode(in); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
