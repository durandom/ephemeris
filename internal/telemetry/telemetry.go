package telemetry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/durandom/ephemeris/internal/inventory"
	"github.com/durandom/ephemeris/internal/timemachine"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

const destinationRole = "time_machine"

type Config struct {
	Enabled      bool
	Endpoint     string
	FlushTimeout time.Duration
	Version      string
	// ErrorLog receives a generic local diagnostic. It intentionally never
	// receives the exporter error because that can contain an endpoint or token.
	ErrorLog func(string)
}

type RunRecord struct {
	Host        string
	Outcome     string
	RC          int
	DurationS   float64
	BytesCopied int64
	Destination string
}

// T is nil-safe. Export failures must never make a backup fail or stop.
type T struct {
	lp       *sdklog.LoggerProvider
	logger   otellog.Logger
	flush    time.Duration
	host     string
	errorLog func(string)
}

func Setup(ctx context.Context, cfg Config) (*T, error) {
	if !cfg.Enabled {
		return &T{}, nil
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:4318"
	}
	if cfg.FlushTimeout <= 0 {
		cfg.FlushTimeout = 5 * time.Second
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.ErrorLog != nil {
		// ephemeris is a short-lived CLI and creates one telemetry setup per
		// process. Redirecting OTel's process-global handler is therefore safe,
		// and prevents SDK errors from escaping to launchd stderr unbounded.
		otel.SetErrorHandler(errorHandler(cfg.ErrorLog))
	}

	res := resource.NewSchemaless(
		attribute.String("service.name", "ephemeris"),
		attribute.String("service.version", controlledVersion(cfg.Version)),
	)

	logExp, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(logsEndpoint(cfg.Endpoint)))
	if err != nil {
		logError(cfg.ErrorLog)
		return &T{}, fmt.Errorf("otlp log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)

	host, _ := os.Hostname()
	return &T{
		lp:       lp,
		logger:   lp.Logger("github.com/durandom/ephemeris"),
		flush:    cfg.FlushTimeout,
		host:     host,
		errorLog: cfg.ErrorLog,
	}, nil
}

func errorHandler(write func(string)) otel.ErrorHandler {
	return otel.ErrorHandlerFunc(func(error) {
		logError(write)
	})
}

func logError(write func(string)) {
	if write != nil {
		write("telemetry export failed")
	}
}

func (t *T) Host() string {
	if t == nil || t.host == "" {
		h, _ := os.Hostname()
		if h == "" {
			return "unknown"
		}
		return h
	}
	return t.host
}

func (t *T) StartPhase(ctx context.Context, _ string) (context.Context, func(rc int)) {
	// Backup telemetry is intentionally logs-only. Exporting traces without a
	// trace pipeline turns successful phases into avoidable exporter errors.
	return ctx, func(int) {}
}

func logsEndpoint(endpoint string) string {
	return strings.TrimRight(endpoint, "/") + "/v1/logs"
}

func (t *T) EmitRun(ctx context.Context, rec RunRecord) {
	if t == nil || t.logger == nil {
		return
	}
	var r otellog.Record
	r.SetEventName("ephemeris.run")
	r.SetBody(otellog.StringValue("ephemeris.run"))
	r.SetSeverity(severityFor(rec.Outcome))
	r.SetTimestamp(time.Now())
	r.AddAttributes(runAttributes(rec)...)
	t.logger.Emit(ctx, r)
}

func (t *T) EmitInventory(ctx context.Context, host string, area inventory.Area) {
	if t == nil || t.logger == nil {
		return
	}
	var r otellog.Record
	r.SetEventName("ephemeris.inventory")
	r.SetBody(otellog.StringValue("ephemeris.inventory"))
	r.SetTimestamp(time.Now())
	r.AddAttributes(inventoryAttributes(host, area)...)
	t.logger.Emit(ctx, r)
}

func (t *T) EmitProgress(ctx context.Context, host string, st timemachine.Status) {
	if t == nil || t.logger == nil {
		return
	}
	var r otellog.Record
	r.SetEventName("ephemeris.progress")
	r.SetBody(otellog.StringValue("ephemeris.progress"))
	r.SetTimestamp(time.Now())
	r.AddAttributes(progressAttributes(host, st)...)
	t.logger.Emit(ctx, r)
}

func runAttributes(rec RunRecord) []otellog.KeyValue {
	return []otellog.KeyValue{
		otellog.String("host", controlledHost(rec.Host)),
		otellog.String("outcome", controlledOutcome(rec.Outcome)),
		otellog.Int("rc", rec.RC),
		otellog.Float64("duration_s", rec.DurationS),
		otellog.Int64("bytes_copied", rec.BytesCopied),
		otellog.String("destination", destinationRole),
	}
}

func inventoryAttributes(host string, area inventory.Area) []otellog.KeyValue {
	return []otellog.KeyValue{
		otellog.String("host", controlledHost(host)),
		otellog.String("area", controlledArea(area.Path)),
		otellog.Int64("files", area.Files),
		otellog.Int64("bytes", area.Bytes),
		otellog.Int64("eperm", area.EPERM),
	}
}

func progressAttributes(host string, st timemachine.Status) []otellog.KeyValue {
	return []otellog.KeyValue{
		otellog.String("host", controlledHost(host)),
		otellog.String("phase", controlledPhase(st.Phase)),
		otellog.Int64("files", st.Files),
		otellog.Int64("total_files", st.TotalFiles),
		otellog.Int64("bytes", st.Bytes),
		otellog.Float64("percent", st.Percent),
		otellog.Float64("raw_percent", st.RawPercent),
		otellog.Float64("time_remaining_s", st.TimeRemaining),
		otellog.String("destination", destinationRole),
	}
}

func controlledVersion(version string) string {
	if version == "dev" {
		return version
	}
	if !strings.HasPrefix(version, "v") {
		return "unknown"
	}
	for _, r := range version[1:] {
		if !(r >= '0' && r <= '9' || r == '.' || r == '-') {
			return "unknown"
		}
	}
	return version
}

func controlledHost(host string) string {
	if strings.EqualFold(strings.TrimSuffix(host, ".local"), "proteus") {
		return "proteus"
	}
	return "unknown"
}

func controlledArea(path string) string {
	path = filepath.Clean(path)
	if home, err := os.UserHomeDir(); err == nil {
		switch path {
		case filepath.Join(home, "src"):
			return "home_src"
		case filepath.Join(home, "Library"):
			return "home_library"
		case filepath.Join(home, "Documents"):
			return "home_documents"
		case filepath.Join(home, "Movies"):
			return "home_movies"
		case filepath.Join(home, "Music"):
			return "home_music"
		case filepath.Join(home, "Pictures"):
			return "home_pictures"
		case filepath.Join(home, "Downloads"):
			return "home_downloads"
		case filepath.Join(home, ".local"):
			return "home_local"
		case filepath.Join(home, ".claude"):
			return "home_claude"
		case filepath.Join(home, ".codex"):
			return "home_codex"
		case filepath.Join(home, ".config"):
			return "home_config"
		}
	}
	switch path {
	case "/Applications":
		return "applications"
	case "/Library":
		return "system_library"
	case "/opt":
		return "opt"
	case "/private/var":
		return "private_var"
	default:
		return "other"
	}
}

func controlledOutcome(outcome string) string {
	switch outcome {
	case "success", "failed", "skipped_no_disk", "already_running":
		return outcome
	default:
		return "unknown"
	}
}

func controlledPhase(phase string) string {
	switch phase {
	case "Copying":
		return "copying"
	case "Preparing":
		return "preparing"
	case "ThinningPreBackup":
		return "thinning"
	case "Finishing":
		return "finishing"
	default:
		return "unknown"
	}
}

func (t *T) Shutdown() {
	if t == nil {
		return
	}
	timeout := t.flush
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if t.lp != nil {
		if err := t.lp.Shutdown(ctx); err != nil {
			logError(t.errorLog)
		}
	}
}

func severityFor(outcome string) otellog.Severity {
	if outcome == "failed" {
		return otellog.SeverityError
	}
	return otellog.SeverityInfo
}
