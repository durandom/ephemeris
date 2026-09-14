package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	cmd := NewRootCommand(BuildInfo{Version: "v0.0.0-test", Commit: "abc", Date: "now"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "v0.0.0-test") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestRunRejectedOffDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("run is allowed on darwin")
	}
	cmd := NewRootCommand(BuildInfo{Version: "dev"})
	cmd.SetArgs([]string{"run"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "macOS") {
		t.Fatalf("err = %v", err)
	}
}
