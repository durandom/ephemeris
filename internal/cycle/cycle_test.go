package cycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/durandom/ephemeris/internal/config"
	"github.com/durandom/ephemeris/internal/inventory"
	"github.com/durandom/ephemeris/internal/timemachine"
)

type fakeHost struct {
	mu         sync.Mutex
	present    bool
	mounted    bool
	running    bool
	statusErr  error
	mountErr   error
	unmountErr error
	backupErr  error
	backupOut  string
	backupWait time.Duration
	mounts     int
	unmounts   int
	backups    int
	statuses   int
	files      int64
}

func (h *fakeHost) DiskPresent(string) bool { return h.present }
func (h *fakeHost) VolumeMounted(string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mounted
}
func (h *fakeHost) Mount(string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mounts++
	if h.mountErr != nil {
		return h.mountErr
	}
	h.mounted = true
	return nil
}
func (h *fakeHost) Unmount(string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.unmounts++
	if h.unmountErr != nil {
		return h.unmountErr
	}
	h.mounted = false
	return nil
}
func (h *fakeHost) Status() (timemachine.Status, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statuses++
	return timemachine.Status{Running: h.running, Phase: "Copying", Files: h.files, TotalFiles: 100}, h.statusErr
}
func (h *fakeHost) StartBackup() (string, error) {
	h.mu.Lock()
	h.backups++
	wait := h.backupWait
	h.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
	return h.backupOut, h.backupErr
}

func testCfg(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		DestinationUUID:  "uuid",
		DestinationName:  "tm2",
		StateDir:         t.TempDir(),
		ScanRoots:        []string{"/tmp/src"},
		ProgressInterval: 0,
		InventoryEvery:   0,
	}
}

func TestRunSkippedNoDiskStillRefreshesExclusions(t *testing.T) {
	var excludes [][]string
	host := &fakeHost{present: false}
	res := Run(context.Background(), testCfg(t), Deps{
		Host: host,
		Exclude: func(args ...string) error {
			cp := append([]string{}, args...)
			excludes = append(excludes, cp)
			return errors.New("catalog failed")
		},
		Hostname: "proteus",
	})
	if res.Outcome != OutcomeSkippedNoDisk {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	if res.RC != 0 {
		t.Fatalf("rc = %d", res.RC)
	}
	if host.backups != 0 || host.mounts != 0 {
		t.Fatalf("must not mount/backup when disk absent")
	}
	if len(excludes) < 2 {
		t.Fatalf("excludes = %#v", excludes)
	}
	if excludes[0][1] != "apply-catalog" {
		t.Fatalf("first = %#v", excludes[0])
	}
	if excludes[1][1] != "scan" {
		t.Fatalf("second = %#v", excludes[1])
	}
}

func TestRunStatusFailureDoesNotTouchBackup(t *testing.T) {
	var logs []string
	host := &fakeHost{present: true, mounted: true, statusErr: errors.New("unavailable")}
	res := Run(context.Background(), testCfg(t), Deps{
		Host: host,
		Log:  func(message string) { logs = append(logs, message) },
	})
	if res.Outcome != OutcomeFailed || res.RC == 0 {
		t.Fatalf("outcome=%s rc=%d", res.Outcome, res.RC)
	}
	if host.mounts != 0 || host.unmounts != 0 || host.backups != 0 {
		t.Fatalf("status failure touched backup: mounts=%d unmounts=%d backups=%d", host.mounts, host.unmounts, host.backups)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "status check failed — not starting backup") {
		t.Fatalf("missing non-sensitive status failure log: %#v", logs)
	}
}

func TestRunAlreadyRunningDoesNotStartOrUnmount(t *testing.T) {
	host := &fakeHost{present: true, mounted: true, running: true}
	res := Run(context.Background(), testCfg(t), Deps{Host: host, Hostname: "proteus"})
	if res.Outcome != OutcomeAlreadyRunning {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	if host.backups != 0 || host.unmounts != 0 {
		t.Fatalf("interrupted a running backup: backups=%d unmounts=%d", host.backups, host.unmounts)
	}
}

func TestRunSuccessMountsParsesAndUnmounts(t *testing.T) {
	host := &fakeHost{
		present:   true,
		mounted:   false,
		backupOut: "Total copied: 1.00 MB (1048576 bytes)\nAvg speed: 1.00 MB/min (1 bytes/sec)\n",
	}
	res := Run(context.Background(), testCfg(t), Deps{Host: host, Hostname: "proteus"})
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("outcome = %s rc=%d", res.Outcome, res.RC)
	}
	if host.mounts != 1 || host.backups != 1 || host.unmounts != 1 {
		t.Fatalf("mounts=%d backups=%d unmounts=%d", host.mounts, host.backups, host.unmounts)
	}
	if res.BytesCopied != 1048576 {
		t.Fatalf("bytes = %d", res.BytesCopied)
	}
	if res.BytesPerSec != 1 {
		t.Fatalf("speed = %d", res.BytesPerSec)
	}
}

func TestRunLogsEveryCompletedPhase(t *testing.T) {
	var logs []string
	host := &fakeHost{
		present:   true,
		mounted:   false,
		backupOut: "Total copied: 1.00 MB (1048576 bytes)\nAvg speed: 1.00 MB/min (1 bytes/sec)\n",
	}
	Run(context.Background(), testCfg(t), Deps{
		Host: host,
		Log: func(msg string) {
			logs = append(logs, msg)
		},
	})
	joined := strings.Join(logs, "\n")
	for _, phase := range []string{"ephemeris.run", "apply_catalog", "scan", "mount", "backup", "unmount"} {
		if !strings.Contains(joined, "→ "+phase) || !strings.Contains(joined, "← "+phase+" (duration=") {
			t.Fatalf("missing completed phase %q in logs:\n%s", phase, joined)
		}
	}
	if !strings.Contains(joined, "backup complete bytes_copied=1048576 bytes_per_sec=1") {
		t.Fatalf("missing backup summary in logs:\n%s", joined)
	}
	if !strings.Contains(joined, "ephemeris run done outcome=success") {
		t.Fatalf("missing final outcome in logs:\n%s", joined)
	}
}

func TestAppendLogRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cycle.log")
	write := appendLog(path, 64)
	write("first entry with enough content to rotate the log")
	write("second entry with enough content to rotate the log")

	previous, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(previous), "first entry") || !strings.Contains(string(current), "second entry") {
		t.Fatalf("rotation contents previous=%q current=%q", previous, current)
	}
}

func TestRunBackupFailureStillUnmounts(t *testing.T) {
	host := &fakeHost{present: true, mounted: true, backupErr: errors.New("tmutil failed")}
	res := Run(context.Background(), testCfg(t), Deps{Host: host})
	if res.Outcome != OutcomeFailed || res.RC == 0 {
		t.Fatalf("outcome=%s rc=%d", res.Outcome, res.RC)
	}
	if host.unmounts != 1 {
		t.Fatalf("unmounts = %d", host.unmounts)
	}
}

func TestRunMountFailureDoesNotBackup(t *testing.T) {
	host := &fakeHost{present: true, mounted: false, mountErr: errors.New("nope")}
	res := Run(context.Background(), testCfg(t), Deps{Host: host})
	if res.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	if host.backups != 0 {
		t.Fatal("started backup after mount failure")
	}
}

func TestInventoryRunsWhenStampMissing(t *testing.T) {
	cfg := testCfg(t)
	cfg.InventoryEvery = time.Hour
	var called atomic.Bool
	host := &fakeHost{present: false}
	Run(context.Background(), cfg, Deps{
		Host: host,
		Inventory: func() ([]inventory.Area, error) {
			called.Store(true)
			return []inventory.Area{{Path: "/tmp", Files: 3, Bytes: 9, EPERM: 1}}, nil
		},
	})
	if !called.Load() {
		t.Fatal("expected inventory")
	}
	if _, err := os.Stat(filepath.Join(cfg.StateDir, lastInventoryFile)); err != nil {
		t.Fatal(err)
	}
}

func TestInventorySkippedWhenFresh(t *testing.T) {
	cfg := testCfg(t)
	cfg.InventoryEvery = time.Hour
	if err := WriteInventoryStamp(cfg.StateDir, time.Now()); err != nil {
		t.Fatal(err)
	}
	host := &fakeHost{present: false}
	var called bool
	Run(context.Background(), cfg, Deps{
		Host: host,
		Inventory: func() ([]inventory.Area, error) {
			called = true
			return nil, nil
		},
	})
	if called {
		t.Fatal("inventory ran too soon")
	}
}

func TestProgressPollsDuringBackup(t *testing.T) {
	cfg := testCfg(t)
	cfg.ProgressInterval = 20 * time.Millisecond
	host := &fakeHost{present: true, mounted: true, backupWait: 80 * time.Millisecond}
	Run(context.Background(), cfg, Deps{Host: host})
	host.mu.Lock()
	n := host.statuses
	host.mu.Unlock()
	if n < 2 {
		t.Fatalf("statuses = %d, want progress polls", n)
	}
}
