package timemachine

import (
	"strings"
	"testing"
)

func TestMountNeedle(t *testing.T) {
	out := "/dev/disk4s2 on /Volumes/tm2 (apfs, local)\n"
	if !strings.Contains(out, MountNeedle("tm2")) {
		t.Fatal("expected match")
	}
	if strings.Contains(out, MountNeedle("tm")) {
		t.Fatal("tm should not match tm2")
	}
}
