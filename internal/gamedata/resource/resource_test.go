package resource

import (
	"os"
	"reflect"
	"testing"
)

// testdata/nav_hulls_presets.vdata_c is a real compiled resource from pak01
// of the game (scripts/nav_hulls_presets.vdata_c): small, with a nested object
// and a string array. It exercises the block container here and the KV3
// decoder underneath.
func readSample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/nav_hulls_presets.vdata_c")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBlocks(t *testing.T) {
	blocks, err := Blocks(readSample(t))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := blocks["DATA"]
	if !ok || len(data) < 4 {
		t.Fatalf("no DATA block; got %d blocks", len(blocks))
	}
	if magic := string(data[1:4]); magic != "3VK" {
		t.Errorf("DATA block magic = %q, want KV3", magic)
	}
}

func TestDecodeData(t *testing.T) {
	obj, err := DecodeData(readSample(t))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"generic_data_type", "default"}; !reflect.DeepEqual(obj.Keys, want) {
		t.Errorf("keys = %q, want %q", obj.Keys, want)
	}
	if got := obj.Str("generic_data_type"); got != "CNavHullPresetVData" {
		t.Errorf("generic_data_type = %q", got)
	}
	hulls, ok := obj.Obj("default").Get("m_vecNavHulls").([]any)
	if want := []any{"default", "trooper", "neutral_large"}; !ok || !reflect.DeepEqual(hulls, want) {
		t.Errorf("m_vecNavHulls = %v, want %v", hulls, want)
	}
}

func TestDecodeDataRejectsBadInput(t *testing.T) {
	sample := readSample(t)
	cases := map[string][]byte{
		"empty":           nil,
		"short":           sample[:10],
		"truncated table": sample[:20],
		"truncated block": sample[:len(sample)/2],
	}
	for name, in := range cases {
		if _, err := DecodeData(in); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestDecodeDataWithoutDataBlock(t *testing.T) {
	// A valid container with zero blocks: file size, versions, block offset 8, count 0.
	file := []byte{16, 0, 0, 0, 12, 0, 1, 0, 8, 0, 0, 0, 0, 0, 0, 0}
	if _, err := DecodeData(file); err == nil {
		t.Error("want error for a resource without a DATA block")
	}
}
