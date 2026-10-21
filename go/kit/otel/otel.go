// Package otel sets up OpenTelemetry traces, metrics and logs for a Synthwerk service
// (D-12, D-36). Services speak only OTLP and do not know the backend.
//
//   - The exporters are OTLP over gRPC. They start only when OTEL_EXPORTER_OTLP_ENDPOINT
//     is set. Without it, Setup installs no exporter and logs one WARN line: tests and
//     plain local runs need no collector.
//   - The SDK reads the standard OTEL_* variables (OTEL_SERVICE_NAME,
//     OTEL_RESOURCE_ATTRIBUTES, OTEL_TRACES_SAMPLER, OTEL_TRACES_SAMPLER_ARG, ...).
//   - The resource holds service.name, service.version, service.namespace=synthwerk,
//     deployment.environment.name and service.instance.id.
//   - The propagator is W3C traceparent + baggage.
package otel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Namespace is the service.namespace of every Synthwerk service.
const Namespace = "synthwerk"

// Config names the service.
type Config struct {
	// ServiceName is the repo name, for example "synthwerk-pulse". OTEL_SERVICE_NAME wins.
	ServiceName string
	// Version is the release version.
	Version string
	// Env is SYNTHWERK_PLATFORM_ENV.
	Env string
}

// Telemetry is the result of Setup.
type Telemetry struct {
	// Exporting is true when OTEL_EXPORTER_OTLP_ENDPOINT is set and the exporters run.
	Exporting bool
	// LoggerProvider feeds the OTel log bridge (log.Options.LoggerProvider).
	// It is a no-op provider when Exporting is false.
	LoggerProvider otellog.LoggerProvider
	shutdown       []func(context.Context) error
}

// Shutdown flushes and stops the providers. Call it once at exit.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for i := len(t.shutdown) - 1; i >= 0; i-- {
		errs = append(errs, t.shutdown[i](ctx))
	}
	return errors.Join(errs...)
}

// Setup installs the global tracer, meter and logger providers and the propagator.
// logger gets the warning when no endpoint is set; it may be nil.
func Setup(ctx context.Context, cfg Config, logger *slog.Logger) (*Telemetry, error) {
	res, err := Resource(ctx, cfg)
	if err != nil {
		return nil, err
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	t := &Telemetry{}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		// Spans still get ids, so logs and events carry trace_id and traceparent.
		tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res))
		otel.SetTracerProvider(tp)
		otel.SetMeterProvider(mp)
		t.LoggerProvider = noop.NewLoggerProvider()
		t.shutdown = append(t.shutdown, tp.Shutdown, mp.Shutdown)
		if logger != nil {
			logger.WarnContext(ctx, "telemetry export is off",
				slog.String("event.name", "otel.exporter.disabled"),
				slog.String("synthwerk.reason", "OTEL_EXPORTER_OTLP_ENDPOINT is not set"))
		}
		return t, nil
	}
	if p := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"); p != "" && p != "grpc" {
		return nil, fmt.Errorf("otel: OTEL_EXPORTER_OTLP_PROTOCOL=%q is not supported, use grpc", p)
	}

	traceExp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("otel: trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(traceExp))
	t.shutdown = append(t.shutdown, tp.Shutdown)

	metricExp, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		_ = t.Shutdown(ctx)
		return nil, fmt.Errorf("otel: metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)))
	t.shutdown = append(t.shutdown, mp.Shutdown)

	logExp, err := otlploggrpc.New(ctx)
	if err != nil {
		_ = t.Shutdown(ctx)
		return nil, fmt.Errorf("otel: log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)))
	t.shutdown = append(t.shutdown, lp.Shutdown)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetLoggerProvider(lp)
	t.LoggerProvider = lp
	t.Exporting = true
	return t, nil
}

// Resource builds the OTel resource. OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES win
// over cfg.
func Resource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	if cfg.ServiceName == "" || cfg.Version == "" || cfg.Env == "" {
		return nil, errors.New("otel: ServiceName, Version and Env are required")
	}
	instance, err := os.Hostname()
	if err != nil || instance == "" {
		instance = uuid.NewString()
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Version),
			semconv.ServiceNamespace(Namespace),
			semconv.DeploymentEnvironmentName(cfg.Env),
			semconv.ServiceInstanceID(instance),
		),
		resource.WithFromEnv(), // after WithAttributes: the env wins
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: resource: %w", err)
	}
	return res, nil
}
