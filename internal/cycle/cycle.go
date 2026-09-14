package cycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/durandom/ephemeris/internal/config"
	"github.com/durandom/ephemeris/internal/inventory"
	"github.com/durandom/ephemeris/internal/telemetry"
	"github.com/durandom/ephemeris/internal/timemachine"
)

const (
	OutcomeSuccess        = "success"
	OutcomeFailed         = "failed"
	OutcomeSkippedNoDisk  = "skipped_no_disk"
	OutcomeAlreadyRunning = "already_running"
	lastInventoryFile     = "last_inventory"

	// MaxCycleLogBytes bounds each retained cycle log. Rotation keeps one
	// previous file, so telemetry failures cannot grow local logs forever.
	MaxCycleLogBytes int64 = 1 << 20
)

type Result struct {
	Outcome     string
	RC          int
	Duration    time.Duration
	BytesCopied int64
	BytesPerSec int64
	Destination string
}

type ExcludeFunc func(args ...string) error

type InventoryFunc func() ([]inventory.Area, error)

type Deps struct {
	Host      timemachine.Host
	Exclude   ExcludeFunc
	Inventory InventoryFunc
	Telemetry *telemetry.T
	Now       func() time.Time
	Hostname  string
	Log       func(string)
}

func Run(ctx context.Context, cfg config.Config, deps Deps) (res Result) {
	started := now(deps)
	res = Result{Destination: cfg.DestinationName, Outcome: OutcomeSuccess}
	defer func() {
		res.Duration = now(deps).Sub(started)
		logf(deps, "=== ephemeris run done outcome=%s duration=%s rc=%d ===", res.Outcome, res.Duration.Round(time.Second), res.RC)
		emitRun(ctx, deps, res)
	}()

	logf(deps, "=== ephemeris run start ===")
	rootCtx, endRoot := startPhase(ctx, deps, "ephemeris.run")
	defer func() { endRoot(res.RC) }()
	ctx = rootCtx

	runExclusions(ctx, cfg, deps)
	if inventoryDue(cfg, deps) {
		runInventory(ctx, cfg, deps)
	}

	if deps.Host == nil {
		res.Outcome = OutcomeFailed
		res.RC = 1
		return res
	}

	st, err := deps.Host.Status()
	if err != nil {
		logf(deps, "status check failed — not starting backup")
		res.Outcome = OutcomeFailed
		res.RC = 1
		return res
	}
	if st.Running {
		logf(deps, "backup already running — not interrupting")
		res.Outcome = OutcomeAlreadyRunning
		return res
	}

	if !deps.Host.DiskPresent(cfg.DestinationUUID) {
		logf(deps, "TM disk not connected — skipping backup")
		res.Outcome = OutcomeSkippedNoDisk
		return res
	}

	if !deps.Host.VolumeMounted(cfg.DestinationName) {
		_, endMount := startPhase(ctx, deps, "mount")
		err := deps.Host.Mount(cfg.DestinationUUID)
		rc := 0
		if err != nil {
			logf(deps, "mount failed: %v", err)
			rc = 1
			res.Outcome = OutcomeFailed
			res.RC = 1
		}
		endMount(rc)
		if err != nil {
			return res
		}
	} else {
		logf(deps, "mount skipped (already mounted)")
	}

	res = runBackup(ctx, cfg, deps, res)

	_, endUnmount := startPhase(ctx, deps, "unmount")
	if err := deps.Host.Unmount(cfg.DestinationUUID); err != nil {
		logf(deps, "unmount failed (something held the volume?): %v", err)
		endUnmount(1)
	} else {
		endUnmount(0)
	}
	return res
}

func runBackup(ctx context.Context, cfg config.Config, deps Deps, res Result) Result {
	ctx, end := startPhase(ctx, deps, "backup")
	pollCtx, stopPoll := context.WithCancel(ctx)
	defer stopPoll()
	if cfg.ProgressInterval > 0 {
		go pollProgress(pollCtx, cfg, deps)
	}

	stdout, err := deps.Host.StartBackup()
	stopPoll()
	parsed := timemachine.ParseBackupOutput(stdout)
	res.BytesCopied = parsed.BytesCopied
	res.BytesPerSec = parsed.BytesPerSec
	if err != nil {
		res.Outcome = OutcomeFailed
		res.RC = timemachine.ExitCode(err)
		if res.RC == 0 {
			res.RC = 1
		}
		logf(deps, "backup failed: %v", err)
	} else {
		res.Outcome = OutcomeSuccess
		res.RC = 0
		logf(deps, "backup complete bytes_copied=%d bytes_per_sec=%d", res.BytesCopied, res.BytesPerSec)
	}
	end(res.RC)
	return res
}

func runExclusions(ctx context.Context, cfg config.Config, deps Deps) {
	ctx, end := startPhase(ctx, deps, "apply_catalog")
	err := callExclude(deps, "tm", "apply-catalog")
	rc := 0
	if err != nil {
		logf(deps, "apply-catalog: %v", err)
		rc = 1
	}
	end(rc)

	ctx, end = startPhase(ctx, deps, "scan")
	rc = 0
	for _, root := range cfg.ScanRoots {
		if err := callExclude(deps, "tm", "scan", root); err != nil {
			logf(deps, "scan %s: %v", root, err)
			rc = 1
		}
	}
	end(rc)
}

func runInventory(ctx context.Context, cfg config.Config, deps Deps) {
	ctx, end := startPhase(ctx, deps, "inventory")
	rc := 0
	if deps.Inventory == nil {
		end(0)
		return
	}
	areas, err := deps.Inventory()
	if err != nil {
		logf(deps, "inventory: %v", err)
		rc = 1
		end(rc)
		return
	}
	host := hostname(deps)
	for _, area := range areas {
		if deps.Telemetry != nil {
			deps.Telemetry.EmitInventory(ctx, host, area)
		}
		logf(deps, "inventory %s files=%d bytes=%d eperm=%d", area.Path, area.Files, area.Bytes, area.EPERM)
	}
	if err := WriteInventoryStamp(cfg.StateDir, now(deps)); err != nil {
		logf(deps, "inventory stamp: %v", err)
	}
	end(rc)
}

func pollProgress(ctx context.Context, cfg config.Config, deps Deps) {
	ticker := time.NewTicker(cfg.ProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st, err := deps.Host.Status()
			if err != nil {
				continue
			}
			if deps.Telemetry != nil {
				deps.Telemetry.EmitProgress(ctx, hostname(deps), st)
			}
		}
	}
}

func callExclude(deps Deps, args ...string) error {
	if deps.Exclude == nil {
		return nil
	}
	return deps.Exclude(args...)
}

func ExecExclude(bin string) ExcludeFunc {
	return func(args ...string) error {
		if bin == "" {
			return fmt.Errorf("mac-exclude path empty")
		}
		if _, err := os.Stat(bin); err != nil {
			return err
		}
		cmd := exec.Command(bin, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}

func DefaultInventory(cfg config.Config) InventoryFunc {
	return func() ([]inventory.Area, error) {
		skip, err := inventory.LoadSkipPaths(cfg.TMPlist)
		if err != nil {
			skip = nil
		}
		return inventory.WalkAreas(cfg.Inventory.Areas, skip), nil
	}
}

func inventoryDue(cfg config.Config, deps Deps) bool {
	if cfg.InventoryEvery <= 0 {
		return false
	}
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, lastInventoryFile))
	if err != nil {
		return true
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	if err != nil {
		return true
	}
	return now(deps).Sub(t) >= cfg.InventoryEvery
}

func WriteInventoryStamp(stateDir string, at time.Time) error {
	if stateDir == "" {
		return nil
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, lastInventoryFile), []byte(at.UTC().Format(time.RFC3339)+"\n"), 0o600)
}

func startPhase(ctx context.Context, deps Deps, name string) (context.Context, func(rc int)) {
	started := now(deps)
	logf(deps, "→ %s", name)
	phaseCtx, endSpan := startTelemetryPhase(ctx, deps, name)
	return phaseCtx, func(rc int) {
		endSpan(rc)
		logf(deps, "← %s (duration=%s, rc=%d)", name, now(deps).Sub(started).Round(time.Second), rc)
	}
}

func startTelemetryPhase(ctx context.Context, deps Deps, name string) (context.Context, func(rc int)) {
	if deps.Telemetry == nil {
		return ctx, func(int) {}
	}
	return deps.Telemetry.StartPhase(ctx, name)
}

func emitRun(ctx context.Context, deps Deps, res Result) {
	if deps.Telemetry == nil {
		return
	}
	deps.Telemetry.EmitRun(ctx, telemetry.RunRecord{
		Host:        hostname(deps),
		Outcome:     res.Outcome,
		RC:          res.RC,
		DurationS:   res.Duration.Seconds(),
		BytesCopied: res.BytesCopied,
		Destination: res.Destination,
	})
}

func hostname(deps Deps) string {
	if deps.Hostname != "" {
		return deps.Hostname
	}
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}

func now(deps Deps) time.Time {
	if deps.Now != nil {
		return deps.Now()
	}
	return time.Now()
}

func logf(deps Deps, format string, args ...any) {
	if deps.Log != nil {
		deps.Log(fmt.Sprintf(format, args...))
	}
}

func AppendLog(stateDir string) func(string) {
	if stateDir == "" {
		return func(string) {}
	}
	return appendLog(filepath.Join(stateDir, "cycle.log"), MaxCycleLogBytes)
}

func appendLog(path string, maxBytes int64) func(string) {
	if maxBytes <= 0 {
		return func(string) {}
	}
	var mu sync.Mutex
	return func(msg string) {
		mu.Lock()
		defer mu.Unlock()

		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return
		}
		line := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
		if int64(len(line)) > maxBytes {
			line = line[len(line)-int(maxBytes):]
		}
		if info, err := os.Stat(path); err == nil && info.Size()+int64(len(line)) > maxBytes {
			_ = os.Remove(path + ".1")
			_ = os.Rename(path, path+".1")
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
}
