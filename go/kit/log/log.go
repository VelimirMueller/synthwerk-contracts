// Package log builds the Synthwerk JSON logger on log/slog
// (plan 04-logging-observability-reporting.md §2).
//
//   - One JSON object per line on stdout: timestamp (RFC 3339 UTC, ms), level, msg,
//     service.name, service.version, deployment.environment.name, and trace_id,
//     span_id, trace_flags when a span is active.
//   - Forbidden keys (password, token, authorization, email, prompt, ...) are dropped
//     before any handler sees them, also inside groups and on the OTel bridge.
//   - With an OTel LoggerProvider the same records also go to OTLP (otelslog bridge).
package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/env"
)

// Forbidden lists the keys that never reach a log line. A key also matches as the
// last dot segment ("http.request.header.authorization") and in any letter case.
var Forbidden = []string{
	"password", "token", "access_token", "refresh_token", "id_token", "authorization",
	"cookie", "set-cookie", "secret", "api_key", "prompt", "messages", "content",
	"email", "image", "body",
}

// Options configure New.
type Options struct {
	// ServiceName is the repo name, for example "synthwerk-pulse".
	ServiceName string
	// Version is the release version of the service.
	Version string
	// Env is SYNTHWERK_PLATFORM_ENV.
	Env string
	// Level is the minimum level. Default INFO.
	Level slog.Level
	// Format is "json" (default) or "console". console only in env "local".
	Format string
	// Writer gets the lines. Default os.Stdout.
	Writer io.Writer
	// LoggerProvider, when set, also sends every record to OTel (OTLP).
	LoggerProvider otellog.LoggerProvider
}

// OptionsFromEnv reads SYNTHWERK_<SERVICE>_LOG_LEVEL, SYNTHWERK_<SERVICE>_LOG_FORMAT
// and SYNTHWERK_PLATFORM_ENV. service is the short name, for example "pulse".
func OptionsFromEnv(service, version string) (Options, error) {
	if !env.ValidService(service) {
		return Options{}, fmt.Errorf("log: service %q is not a lowercase short name", service)
	}
	platform, err := env.Platform()
	if err != nil {
		return Options{}, fmt.Errorf("log: %w", err)
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(env.Get(service, "LOG_LEVEL", "INFO"))); err != nil {
		return Options{}, fmt.Errorf("log: %s: %w", env.Key(service, "LOG_LEVEL"), err)
	}
	return Options{
		ServiceName: "synthwerk-" + service,
		Version:     version,
		Env:         platform,
		Level:       level,
		Format:      env.Get(service, "LOG_FORMAT", "json"),
	}, nil
}

// New returns the logger. It fails on an unknown format or on console outside "local".
func New(o Options) (*slog.Logger, error) {
	w := o.Writer
	if w == nil {
		w = os.Stdout
	}
	opts := &slog.HandlerOptions{Level: o.Level, ReplaceAttr: replaceTime}
	var out slog.Handler
	switch o.Format {
	case "", "json":
		out = slog.NewJSONHandler(w, opts)
	case "console":
		if o.Env != "local" {
			return nil, fmt.Errorf("log: format console is only allowed in env local, not %q", o.Env)
		}
		out = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("log: format %q is not json or console", o.Format)
	}
	// The OTel resource carries these fields for OTLP. stdout needs them per line.
	out = traceHandler{out.WithAttrs([]slog.Attr{
		slog.String("service.name", o.ServiceName),
		slog.String("service.version", o.Version),
		slog.String("deployment.environment.name", o.Env),
	})}

	h := slog.Handler(out)
	if o.LoggerProvider != nil {
		bridge := otelslog.NewHandler(o.ServiceName,
			otelslog.WithLoggerProvider(o.LoggerProvider), otelslog.WithVersion(o.Version))
		h = slog.NewMultiHandler(out, levelHandler{bridge, o.Level})
	}
	return slog.New(redactHandler{h}), nil
}

// replaceTime writes the time as "timestamp" in RFC 3339 UTC with milliseconds.
func replaceTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
		return slog.String("timestamp", a.Value.Time().UTC().Format("2006-01-02T15:04:05.000Z07:00"))
	}
	return a
}

// IsForbidden reports whether key must never be logged.
func IsForbidden(key string) bool {
	k := strings.ToLower(key)
	if i := strings.LastIndexByte(k, '.'); i >= 0 {
		k = k[i+1:]
	}
	for _, f := range Forbidden {
		if k == f {
			return true
		}
	}
	return false
}

// redactHandler drops forbidden keys before the record reaches any handler.
type redactHandler struct{ next slog.Handler }

func (h redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h redactHandler) Handle(ctx context.Context, r slog.Record) error {
	clean := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if a, ok := redact(a); ok {
			clean.AddAttrs(a)
		}
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactHandler{h.next.WithAttrs(redactAll(attrs))}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{h.next.WithGroup(name)}
}

func redact(a slog.Attr) (slog.Attr, bool) {
	if IsForbidden(a.Key) {
		return slog.Attr{}, false
	}
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(redactAll(v.Group())...)}, true
	}
	return slog.Attr{Key: a.Key, Value: v}, true
}

func redactAll(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if a, ok := redact(a); ok {
			out = append(out, a)
		}
	}
	return out
}

// traceHandler adds trace_id, span_id and trace_flags of the active span.
type traceHandler struct{ next slog.Handler }

func (h traceHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r = r.Clone()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
			slog.String("trace_flags", sc.TraceFlags().String()),
		)
	}
	return h.next.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.next.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.next.WithGroup(name)}
}

// levelHandler applies the minimum level to a handler that has no level option.
type levelHandler struct {
	next  slog.Handler
	level slog.Level
}

func (h levelHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= h.level && h.next.Enabled(ctx, l)
}

func (h levelHandler) Handle(ctx context.Context, r slog.Record) error { return h.next.Handle(ctx, r) }

func (h levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelHandler{h.next.WithAttrs(attrs), h.level}
}

func (h levelHandler) WithGroup(name string) slog.Handler {
	return levelHandler{h.next.WithGroup(name), h.level}
}
