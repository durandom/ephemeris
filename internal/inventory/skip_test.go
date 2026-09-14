package inventory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkipPathsStripsSlash(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "skip.plist"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSkipPaths(data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"/Users/mhild/.cache": true,
		"/opt/homebrew":       true,
	}
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected %q in %#v", p, got)
		}
	}
}

func TestLoadSkipPathsMissing(t *testing.T) {
	_, err := LoadSkipPaths(filepath.Join(t.TempDir(), "nope.plist"))
	if err == nil {
		t.Fatal("expected error")
	}
}
