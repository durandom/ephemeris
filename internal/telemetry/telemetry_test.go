package telemetry

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/durandom/ephemeris/internal/inventory"
	"github.com/durandom/ephemeris/internal/timemachine"
	"go.opentelemetry.io/otel"
	otellog "go.opentelemetry.io/otel/log"
)

func TestNilSafe(t *testing.T) {
	var tel *T
	ctx, end := tel.StartPhase(context.Background(), "backup")
	end(0)
	tel.EmitRun(ctx, RunRecord{Outcome: "success"})
	tel.EmitInventory(ctx, "host", inventory.Area{Path: "/tmp"})
	tel.EmitProgress(ctx, "host", timemachine.Status{Running: true})
	tel.Shutdown()
}

func TestSetupRoutesOTelErrorsToGenericLocalLog(t *testing.T) {
	var messages []string
	tel, err := Setup(context.Background(), Config{
		Enabled:  true,
		Endpoint: "http://127.0.0.1:1",
		ErrorLog: func(message string) { messages = append(messages, message) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown()

	otel.Handle(errors.New("Post https://token@example.test:4318/v1/logs: connection refused"))
	if len(messages) != 1 || messages[0] != "telemetry export failed" {
		t.Fatalf("messages = %#v", messages)
	}
	if strings.Contains(messages[0], "example.test") || strings.Contains(messages[0], "token") {
		t.Fatalf("message leaked exporter details: %q", messages[0])
	}
}

func TestTelemetryAttributesUseControlledIdentifiers(t *testing.T) {
	const sensitive = "alice@example.test"
	attrs := append([]otellog.KeyValue{}, runAttributes(RunRecord{
		Host:        sensitive,
		Outcome:     sensitive,
		Destination: "tm2-" + sensitive,
	})...)
	attrs = append(attrs, inventoryAttributes(sensitive, inventory.Area{
		Path: "/Users/" + sensitive + "/Documents",
	})...)
	attrs = append(attrs, progressAttributes(sensitive, timemachine.Status{
		Phase:                 sensitive,
		DestinationMountPoint: "/Volumes/" + sensitive,
	})...)

	for _, attr := range attrs {
		if strings.Contains(attr.Value.String(), sensitive) {
			t.Fatalf("telemetry attribute leaked sensitive input: %s", attr)
		}
	}
	if got := attributeValue(runAttributes(RunRecord{Destination: sensitive}), "destination"); got != destinationRole {
		t.Fatalf("run destination = %q", got)
	}
	if got := attributeValue(inventoryAttributes("proteus", inventory.Area{Path: "/tmp/" + sensitive}), "area"); got != "other" {
		t.Fatalf("inventory area = %q", got)
	}
	if got := attributeValue(progressAttributes("proteus", timemachine.Status{Phase: sensitive, DestinationMountPoint: "/Volumes/" + sensitive}), "phase"); got != "unknown" {
		t.Fatalf("progress phase = %q", got)
	}
	if got := attributeValue(progressAttributes("proteus", timemachine.Status{DestinationMountPoint: "/Volumes/" + sensitive}), "destination"); got != destinationRole {
		t.Fatalf("progress destination = %q", got)
	}
	if got := controlledVersion("v1.2.3-" + sensitive); got != "unknown" {
		t.Fatalf("version = %q", got)
	}
}

func attributeValue(attrs []otellog.KeyValue, key string) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value.String()
		}
	}
	return ""
}

func TestDisabledSetup(t *testing.T) {
	tel, err := Setup(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	ctx, end := tel.StartPhase(context.Background(), "scan")
	end(1)
	tel.EmitRun(ctx, RunRecord{Outcome: "failed", RC: 2})
	tel.Shutdown()
}
