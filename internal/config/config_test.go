package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingUsesDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DestinationUUID != DefaultDestinationUUID {
		t.Fatalf("uuid = %q", cfg.DestinationUUID)
	}
	if !cfg.OTel.Enabled {
		t.Fatal("expected otel enabled by default")
	}
	if cfg.ProgressInterval != DefaultProgressEvery {
		t.Fatalf("progress = %s", cfg.ProgressInterval)
	}
}

func TestLoadExplicitMissingErrors(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
destination_uuid = "AAAA"
destination_name = "tm-test"
mac_exclude = "/bin/mac-exclude"
scan_roots = ["/tmp/src"]
state_dir = "/tmp/state"
progress_interval = "10s"
inventory_every = "1h"
tm_plist = "/tmp/tm.plist"

[otel]
enabled = false
endpoint = "http://127.0.0.1:4318"
flush_timeout = "2s"

[inventory]
areas = ["/tmp/a", "~/src"]
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DestinationUUID != "AAAA" || cfg.DestinationName != "tm-test" {
		t.Fatalf("destination = %s %s", cfg.DestinationUUID, cfg.DestinationName)
	}
	if cfg.OTel.Enabled {
		t.Fatal("expected otel disabled")
	}
	if cfg.ProgressInterval != 10*time.Second {
		t.Fatalf("progress = %s", cfg.ProgressInterval)
	}
	if cfg.InventoryEvery != time.Hour {
		t.Fatalf("inventory every = %s", cfg.InventoryEvery)
	}
	if len(cfg.Inventory.Areas) != 2 {
		t.Fatalf("areas = %#v", cfg.Inventory.Areas)
	}
	if cfg.Inventory.Areas[1] == "~/src" {
		t.Fatal("expected ~ expansion")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpandPath("~/foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, "foo") {
		t.Fatalf("got %q", got)
	}
}
