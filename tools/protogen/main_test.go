package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	in := strings.Join([]string{
		`option additional_includes = "basetypes.h";`,
		`option (force_php_generation) = true;`,
		`option optimize_for = SPEED;`,
		`message M {`,
		`	optional uint32 a = 1 [boxed_type = "X_t"];`,
		`	optional uint32 b = 2 [default = 65535, boxed_type = "Y_t", synthetic_default = "65535"];`,
		`	repeated uint32 c = 3 [packed = true, boxed_type = "Z"];`,
		`	optional string d = 4 [default = "a,b"];`,
		`}`,
		`enum E { k_A = 0 [(schema_friendly_name) = "A, really"]; }`,
	}, "\n")
	want := strings.Join([]string{
		`option optimize_for = SPEED;`,
		`message M {`,
		`	optional uint32 a = 1;`,
		`	optional uint32 b = 2 [default = 65535];`,
		`	repeated uint32 c = 3 [packed = true];`,
		`	optional string d = 4 [default = "a,b"];`,
		`}`,
		`enum E { k_A = 0; }`,
		``,
	}, "\n")

	dir := t.TempDir()
	src, dst := filepath.Join(dir, "in.proto"), filepath.Join(dir, "out.proto")
	if err := os.WriteFile(src, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sanitize(src, dst); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
