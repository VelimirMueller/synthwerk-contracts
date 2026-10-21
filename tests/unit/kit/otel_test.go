package kit_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"

	kitotel "github.com/VelimirMueller/synthwerk-contracts/go/kit/otel"
)

func TestOTel(t *testing.T) {
	cfg := kitotel.Config{ServiceName: "synthwerk-example", Version: "0.1.0", Env: "ci"}

	t.Run("Setup - endpoint not set - no exporter, one warning", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		logger, buf := newLogger(t, slog.LevelDebug)
		tel, err := kitotel.Setup(context.Background(), cfg, logger)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })
		if tel.Exporting {
			t.Fatal("Exporting is true without an endpoint")
		}
		got := lines(t, buf)
		if len(got) != 1 || got[0]["level"] != "WARN" || got[0]["event.name"] != "otel.exporter.disabled" {
			t.Fatalf("lines %v, want one WARN otel.exporter.disabled", got)
		}
	})

	t.Run("Setup - protocol http - error", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4317")
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
		if _, err := kitotel.Setup(context.Background(), cfg, nil); err == nil || !strings.Contains(err.Error(), "grpc") {
			t.Fatalf("err %v, want an error that names grpc", err)
		}
	})

	t.Run("Resource - any env - synthwerk attributes, OTEL_SERVICE_NAME wins", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "synthwerk-override")
		res, err := kitotel.Resource(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		want := map[attribute.Key]string{
			"service.name": "synthwerk-override", "service.version": "0.1.0",
			"service.namespace": "synthwerk", "deployment.environment.name": "ci",
		}
		set := res.Set()
		for k, v := range want {
			got, ok := set.Value(k)
			if !ok || got.AsString() != v {
				t.Errorf("%s = %q, want %q", k, got.AsString(), v)
			}
		}
		if v, ok := set.Value("service.instance.id"); !ok || v.AsString() == "" {
			t.Error("service.instance.id is missing")
		}
	})
}
