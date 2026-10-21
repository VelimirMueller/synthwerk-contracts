package kit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	kitlog "github.com/VelimirMueller/synthwerk-contracts/go/kit/log"
)

var timestampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

func newLogger(t *testing.T, level slog.Level) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger, err := kitlog.New(kitlog.Options{
		ServiceName: "synthwerk-example", Version: "0.1.0", Env: "ci", Level: level, Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	return logger, &buf
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("line is not JSON: %v\n%s", err, l)
		}
		out = append(out, m)
	}
	return out
}

// keys returns every key of m, nested keys as "group.key".
func keys(m map[string]any, prefix string) []string {
	var out []string
	for k, v := range m {
		out = append(out, prefix+k)
		if sub, ok := v.(map[string]any); ok {
			out = append(out, keys(sub, prefix+k+".")...)
		}
	}
	return out
}

func TestLog(t *testing.T) {
	t.Run("log line - INFO record - required fields", func(t *testing.T) {
		logger, buf := newLogger(t, slog.LevelInfo)
		logger.Info("thing completed", slog.String("event.name", "example.thing.completed"))
		got := lines(t, buf)
		if len(got) != 1 {
			t.Fatalf("%d lines, want 1", len(got))
		}
		l := got[0]
		want := map[string]any{
			"level": "INFO", "msg": "thing completed", "event.name": "example.thing.completed",
			"service.name": "synthwerk-example", "service.version": "0.1.0", "deployment.environment.name": "ci",
		}
		for k, v := range want {
			if l[k] != v {
				t.Errorf("%s = %v, want %v", k, l[k], v)
			}
		}
		if ts, _ := l["timestamp"].(string); !timestampRe.MatchString(ts) {
			t.Errorf("timestamp %q is not RFC 3339 UTC with ms", ts)
		}
		if _, ok := l["time"]; ok {
			t.Error("line has key time, want timestamp only")
		}
	})

	t.Run("log line - forbidden keys anywhere - keys dropped", func(t *testing.T) {
		logger, buf := newLogger(t, slog.LevelInfo)
		logger.With(slog.String("password", "p")).WithGroup("req").Info("request",
			slog.String("Authorization", "Bearer x"),
			slog.String("http.request.header.cookie", "c"),
			slog.Group("user", slog.String("email", "a@b.c"), slog.String("id", "u_h:1")),
			slog.String("ok_field", "kept"))
		l := lines(t, buf)[0]
		for _, k := range keys(l, "") {
			if kitlog.IsForbidden(k) {
				t.Errorf("forbidden key %q in line %v", k, l)
			}
		}
		if !strings.Contains(buf.String(), `"ok_field":"kept"`) || !strings.Contains(buf.String(), `"id":"u_h:1"`) {
			t.Errorf("allowed fields missing: %s", buf.String())
		}
		for _, secret := range []string{"Bearer x", "a@b.c", `"p"`} {
			if strings.Contains(buf.String(), secret) {
				t.Errorf("value %s leaked: %s", secret, buf.String())
			}
		}
	})

	t.Run("log line - span active - trace_id and span_id", func(t *testing.T) {
		logger, buf := newLogger(t, slog.LevelInfo)
		tp := sdktrace.NewTracerProvider()
		t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
		ctx, span := tp.Tracer("test").Start(context.Background(), "op")
		logger.InfoContext(ctx, "inside span")
		span.End()
		l := lines(t, buf)[0]
		if l["trace_id"] != span.SpanContext().TraceID().String() || l["span_id"] != span.SpanContext().SpanID().String() {
			t.Errorf("trace fields %v / %v do not match the span", l["trace_id"], l["span_id"])
		}
	})

	t.Run("New - console outside local - error", func(t *testing.T) {
		if _, err := kitlog.New(kitlog.Options{Env: "prd", Format: "console"}); err == nil {
			t.Fatal("no error for console in prd")
		}
	})

	t.Run("New - unknown format - error", func(t *testing.T) {
		if _, err := kitlog.New(kitlog.Options{Env: "local", Format: "xml"}); err == nil {
			t.Fatal("no error for format xml")
		}
	})

	t.Run("OptionsFromEnv - variable without service prefix - ignored", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "DEBUG")
		t.Setenv("SYNTHWERK_LOG_LEVEL", "DEBUG")
		t.Setenv("SYNTHWERK_OTHER_LOG_LEVEL", "DEBUG")
		t.Setenv("SYNTHWERK_PLATFORM_ENV", "stg")
		o, err := kitlog.OptionsFromEnv("example", "0.1.0")
		if err != nil {
			t.Fatal(err)
		}
		if o.Level != slog.LevelInfo || o.Env != "stg" || o.ServiceName != "synthwerk-example" {
			t.Fatalf("options %+v, want level INFO, env stg, name synthwerk-example", o)
		}
		t.Setenv("SYNTHWERK_EXAMPLE_LOG_LEVEL", "WARN")
		if o, _ = kitlog.OptionsFromEnv("example", "0.1.0"); o.Level != slog.LevelWarn {
			t.Fatalf("level %v, want WARN from SYNTHWERK_EXAMPLE_LOG_LEVEL", o.Level)
		}
	})

	t.Run("OptionsFromEnv - unknown platform env - error", func(t *testing.T) {
		t.Setenv("SYNTHWERK_PLATFORM_ENV", "production")
		if _, err := kitlog.OptionsFromEnv("example", "0.1.0"); err == nil {
			t.Fatal("no error for SYNTHWERK_PLATFORM_ENV=production")
		}
	})
}

func TestLogHTTP(t *testing.T) {
	serve := func(t *testing.T, level slog.Level, req *http.Request) (*httptest.ResponseRecorder, []map[string]any) {
		t.Helper()
		logger, buf := newLogger(t, level)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /v1/targets/{name}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
		mux.HandleFunc("GET /healthz", func(http.ResponseWriter, *http.Request) {})
		rec := httptest.NewRecorder()
		kitlog.HTTP(logger, mux).ServeHTTP(rec, req)
		return rec, lines(t, buf)
	}

	t.Run("HTTP - request completed - one line with route template", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/targets/identity?token=x", nil)
		req.RemoteAddr = "203.0.113.77:5555"
		rec, got := serve(t, slog.LevelInfo, req)
		if len(got) != 1 {
			t.Fatalf("%d lines, want 1", len(got))
		}
		l := got[0]
		if l["http.route"] != "/v1/targets/{name}" || l["http.response.status_code"] != float64(418) ||
			l["http.request.method"] != "GET" || l["client.address"] != "203.0.113.0" ||
			l["event.name"] != "http.request.completed" {
			t.Errorf("line %v", l)
		}
		if l["synthwerk.request_id"] != rec.Header().Get(kitlog.RequestIDHeader) || rec.Header().Get(kitlog.RequestIDHeader) == "" {
			t.Errorf("request id %v not echoed (%q)", l["synthwerk.request_id"], rec.Header().Get(kitlog.RequestIDHeader))
		}
	})

	t.Run("HTTP - valid caller request id - kept", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/targets/identity", nil)
		req.Header.Set(kitlog.RequestIDHeader, "req_abc.1-2")
		rec, _ := serve(t, slog.LevelInfo, req)
		if got := rec.Header().Get(kitlog.RequestIDHeader); got != "req_abc.1-2" {
			t.Fatalf("request id %q, want req_abc.1-2", got)
		}
	})

	t.Run("HTTP - unsafe caller request id - replaced", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/targets/identity", nil)
		req.Header.Set(kitlog.RequestIDHeader, "<script>")
		rec, _ := serve(t, slog.LevelInfo, req)
		if got := rec.Header().Get(kitlog.RequestIDHeader); got == "<script>" || !strings.HasPrefix(got, "req_") {
			t.Fatalf("request id %q, want a new req_ id", got)
		}
	})

	t.Run("HTTP - probe request at INFO - no line", func(t *testing.T) {
		_, got := serve(t, slog.LevelInfo, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))
		if len(got) != 0 {
			t.Fatalf("%d lines for /healthz at INFO, want 0", len(got))
		}
	})
}
