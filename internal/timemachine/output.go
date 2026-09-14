package timemachine

import (
	"errors"
	"os/exec"
	"regexp"
	"strconv"
)

var (
	totalCopiedRe = regexp.MustCompile(`(?i)Total copied:.*\((\d+)\s*bytes\)`)
	avgSpeedRe    = regexp.MustCompile(`(?i)Avg speed:.*\((\d+)\s*bytes/sec\)`)
)

// BackupOutput is parsed from `tmutil startbackup --block` stdout.
type BackupOutput struct {
	BytesCopied int64
	BytesPerSec int64
}

func ParseBackupOutput(stdout string) BackupOutput {
	var out BackupOutput
	if m := totalCopiedRe.FindStringSubmatch(stdout); len(m) == 2 {
		out.BytesCopied, _ = strconv.ParseInt(m[1], 10, 64)
	}
	if m := avgSpeedRe.FindStringSubmatch(stdout); len(m) == 2 {
		out.BytesPerSec, _ = strconv.ParseInt(m[1], 10, 64)
	}
	return out
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 1
}
