package timemachine

import (
	"errors"
	"os/exec"
	"testing"
)

func TestParseBackupOutput(t *testing.T) {
	stdout := "Starting standard backup\n" +
		"Total copied: 9002.97 MB (9440301056 bytes)\n" +
		"Avg speed:    1090.30 MB/min (19054422 bytes/sec)\n"
	out := ParseBackupOutput(stdout)
	if out.BytesCopied != 9440301056 {
		t.Fatalf("bytes = %d", out.BytesCopied)
	}
	if out.BytesPerSec != 19054422 {
		t.Fatalf("speed = %d", out.BytesPerSec)
	}
}

func TestParseBackupOutputEmpty(t *testing.T) {
	out := ParseBackupOutput("no numbers here")
	if out.BytesCopied != 0 || out.BytesPerSec != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestExitCode(t *testing.T) {
	if ExitCode(nil) != 0 {
		t.Fatal("nil")
	}
	if ExitCode(errors.New("boom")) != 1 {
		t.Fatal("plain error")
	}
	cmd := exec.Command("false")
	err := cmd.Run()
	if ExitCode(err) != 1 {
		t.Fatalf("false rc = %d", ExitCode(err))
	}
}
