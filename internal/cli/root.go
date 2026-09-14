package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"text/tabwriter"
	"time"

	"github.com/durandom/ephemeris/internal/config"
	"github.com/durandom/ephemeris/internal/cycle"
	"github.com/durandom/ephemeris/internal/inventory"
	"github.com/durandom/ephemeris/internal/telemetry"
	"github.com/durandom/ephemeris/internal/timemachine"
	"github.com/spf13/cobra"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

func Execute(build BuildInfo) int {
	if err := NewRootCommand(build).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func NewRootCommand(build BuildInfo) *cobra.Command {
	var configPath string
	root := &cobra.Command{
		Use:           "ephemeris",
		Short:         "Run host backups and report their outcome to the estate telemetry sink",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "config file path")
	root.AddCommand(newVersionCommand(build))
	root.AddCommand(newRunCommand(build, &configPath))
	root.AddCommand(newInventoryCommand(build, &configPath))
	return root
}

func newVersionCommand(build BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "ephemeris %s (commit %s, %s)\n", build.Version, build.Commit, build.Date)
		},
	}
}

func newRunCommand(build BuildInfo, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Refresh exclusions, optionally inventory, then mount-backup-unmount",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "darwin" {
				return fmt.Errorf("ephemeris run is only supported on macOS")
			}
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			localLog := cycle.AppendLog(cfg.StateDir)
			tel := setupTelemetry(cmd.Context(), cfg, build, localLog)
			defer tel.Shutdown()

			res := cycle.Run(cmd.Context(), cfg, cycle.Deps{
				Host:      timemachine.ExecHost{},
				Exclude:   cycle.ExecExclude(cfg.MacExclude),
				Inventory: cycle.DefaultInventory(cfg),
				Telemetry: tel,
				Log:       localLog,
			})
			if res.Outcome == cycle.OutcomeFailed {
				return fmt.Errorf("outcome=%s rc=%d", res.Outcome, res.RC)
			}
			return nil
		},
	}
}

func newInventoryCommand(build BuildInfo, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "inventory",
		Short: "Count files that Time Machine would back up, per area",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			localLog := cycle.AppendLog(cfg.StateDir)
			tel := setupTelemetry(cmd.Context(), cfg, build, localLog)
			defer tel.Shutdown()

			skip, err := inventory.LoadSkipPaths(cfg.TMPlist)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "SkipPaths unavailable: %v\n", err)
				skip = nil
			}
			areas := inventory.WalkAreas(cfg.Inventory.Areas, skip)
			host := tel.Host()
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "files\tGB\teperm\tarea\n")
			var totalF, totalB, totalE int64
			for _, a := range areas {
				tel.EmitInventory(cmd.Context(), host, a)
				fmt.Fprintf(w, "%d\t%.1f\t%d\t%s\n", a.Files, float64(a.Bytes)/1e9, a.EPERM, a.Path)
				totalF += a.Files
				totalB += a.Bytes
				totalE += a.EPERM
			}
			fmt.Fprintf(w, "%d\t%.1f\t%d\tSUM\n", totalF, float64(totalB)/1e9, totalE)
			if err := w.Flush(); err != nil {
				return err
			}
			return cycle.WriteInventoryStamp(cfg.StateDir, time.Now())
		},
	}
}

func setupTelemetry(ctx context.Context, cfg config.Config, build BuildInfo, errorLog func(string)) *telemetry.T {
	tel, err := telemetry.Setup(ctx, telemetry.Config{
		Enabled:      cfg.OTel.Enabled,
		Endpoint:     cfg.OTel.Endpoint,
		FlushTimeout: cfg.OTel.FlushTimeout,
		Version:      build.Version,
		ErrorLog:     errorLog,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "telemetry disabled: setup failed")
		return &telemetry.T{}
	}
	if tel == nil {
		return &telemetry.T{}
	}
	return tel
}
