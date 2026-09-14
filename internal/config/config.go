package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	appDir                = "ephemeris"
	configFile            = "config.toml"
	DefaultProgressEvery  = 60 * time.Second
	DefaultInventoryEvery = 30 * 24 * time.Hour
	DefaultFlushTimeout   = 5 * time.Second
	DefaultTMPlist        = "/Library/Preferences/com.apple.TimeMachine.plist"
)

// Default proteus destination. Override in the config file; do not hard-code
// elsewhere. The frozen `tm` archive volume is not a destination.
const (
	DefaultDestinationUUID = "C4711353-63FC-476E-B57B-AD997762DB96"
	DefaultDestinationName = "tm2"
)

type Config struct {
	DestinationUUID  string
	DestinationName  string
	MacExclude       string
	ScanRoots        []string
	StateDir         string
	ProgressInterval time.Duration
	InventoryEvery   time.Duration
	TMPlist          string
	OTel             OTelConfig
	Inventory        InventoryConfig
}

type OTelConfig struct {
	Enabled      bool
	Endpoint     string
	FlushTimeout time.Duration
}

type InventoryConfig struct {
	Areas []string
}

type fileConfig struct {
	DestinationUUID  string        `toml:"destination_uuid"`
	DestinationName  string        `toml:"destination_name"`
	MacExclude       string        `toml:"mac_exclude"`
	ScanRoots        []string      `toml:"scan_roots"`
	StateDir         string        `toml:"state_dir"`
	ProgressInterval string        `toml:"progress_interval"`
	InventoryEvery   string        `toml:"inventory_every"`
	TMPlist          string        `toml:"tm_plist"`
	OTel             fileOTel      `toml:"otel"`
	Inventory        fileInventory `toml:"inventory"`
}

type fileOTel struct {
	Enabled      *bool  `toml:"enabled"`
	Endpoint     string `toml:"endpoint"`
	FlushTimeout string `toml:"flush_timeout"`
}

type fileInventory struct {
	Areas []string `toml:"areas"`
}

func Default() Config {
	home := homeDir()
	return Config{
		DestinationUUID:  DefaultDestinationUUID,
		DestinationName:  DefaultDestinationName,
		MacExclude:       filepath.Join(home, "bin", "mac-exclude"),
		ScanRoots:        []string{filepath.Join(home, "src")},
		StateDir:         filepath.Join(XDGStateHome(), appDir),
		ProgressInterval: DefaultProgressEvery,
		InventoryEvery:   DefaultInventoryEvery,
		TMPlist:          DefaultTMPlist,
		OTel: OTelConfig{
			Enabled:      true,
			Endpoint:     "http://localhost:4318",
			FlushTimeout: DefaultFlushTimeout,
		},
		Inventory: InventoryConfig{Areas: defaultAreas(home)},
	}
}

func defaultAreas(home string) []string {
	return []string{
		filepath.Join(home, "src"),
		filepath.Join(home, "Library"),
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Movies"),
		filepath.Join(home, "Music"),
		filepath.Join(home, "Pictures"),
		filepath.Join(home, "Downloads"),
		filepath.Join(home, ".local"),
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".config"),
		"/Applications",
		"/Library",
		"/opt",
		"/private/var",
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath()
		expanded, err := ExpandPath(path)
		if err != nil {
			return Config{}, err
		}
		data, err := os.ReadFile(expanded)
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		return parse(cfg, data)
	}

	expanded, err := ExpandPath(path)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(expanded)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return parse(cfg, data)
}

func parse(cfg Config, data []byte) (Config, error) {
	var fc fileConfig
	if err := toml.Unmarshal(data, &fc); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	var err error
	if fc.DestinationUUID != "" {
		cfg.DestinationUUID = fc.DestinationUUID
	}
	if fc.DestinationName != "" {
		cfg.DestinationName = fc.DestinationName
	}
	if fc.MacExclude != "" {
		cfg.MacExclude, err = ExpandPath(fc.MacExclude)
		if err != nil {
			return Config{}, err
		}
	}
	if len(fc.ScanRoots) > 0 {
		cfg.ScanRoots = make([]string, len(fc.ScanRoots))
		for i, root := range fc.ScanRoots {
			cfg.ScanRoots[i], err = ExpandPath(root)
			if err != nil {
				return Config{}, err
			}
		}
	}
	if fc.StateDir != "" {
		cfg.StateDir, err = ExpandPath(fc.StateDir)
		if err != nil {
			return Config{}, err
		}
	}
	if fc.ProgressInterval != "" {
		cfg.ProgressInterval, err = time.ParseDuration(fc.ProgressInterval)
		if err != nil {
			return Config{}, fmt.Errorf("parse progress_interval: %w", err)
		}
	}
	if fc.InventoryEvery != "" {
		cfg.InventoryEvery, err = time.ParseDuration(fc.InventoryEvery)
		if err != nil {
			return Config{}, fmt.Errorf("parse inventory_every: %w", err)
		}
	}
	if fc.TMPlist != "" {
		cfg.TMPlist, err = ExpandPath(fc.TMPlist)
		if err != nil {
			return Config{}, err
		}
	}
	if fc.OTel.Enabled != nil {
		cfg.OTel.Enabled = *fc.OTel.Enabled
	}
	if fc.OTel.Endpoint != "" {
		cfg.OTel.Endpoint = fc.OTel.Endpoint
	}
	if fc.OTel.FlushTimeout != "" {
		cfg.OTel.FlushTimeout, err = time.ParseDuration(fc.OTel.FlushTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("parse otel.flush_timeout: %w", err)
		}
	}
	if len(fc.Inventory.Areas) > 0 {
		cfg.Inventory.Areas = make([]string, len(fc.Inventory.Areas))
		for i, area := range fc.Inventory.Areas {
			cfg.Inventory.Areas[i], err = ExpandPath(area)
			if err != nil {
				return Config{}, err
			}
		}
	}
	return cfg, nil
}

func DefaultPath() string {
	return filepath.Join(XDGConfigHome(), appDir, configFile)
}

func XDGConfigHome() string {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return value
	}
	return filepath.Join(homeDir(), ".config")
}

func XDGStateHome() string {
	if value := os.Getenv("XDG_STATE_HOME"); value != "" {
		return value
	}
	return filepath.Join(homeDir(), ".local", "state")
}

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

func ExpandPath(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	if len(path) > 1 && path[1] != '/' {
		return "", fmt.Errorf("unsupported home path %q", path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if len(path) == 1 {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}
