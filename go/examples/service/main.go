// Command service is the example Synthwerk Go service. See package app and
// docs/features/go-service-kit.md.
//
// Environment:
//
//	SYNTHWERK_PLATFORM_ENV          local (default), ci, dev, stg, prd
//	SYNTHWERK_EXAMPLE_HTTP_ADDR     listen address, default :8080
//	SYNTHWERK_EXAMPLE_DATABASE_URL  Postgres URL (required, secret)
//	SYNTHWERK_EXAMPLE_NATS_URL      NATS URL, default nats://127.0.0.1:4222
//	SYNTHWERK_EXAMPLE_LOG_LEVEL     DEBUG, INFO (default), WARN, ERROR
//	SYNTHWERK_EXAMPLE_LOG_FORMAT    json (default) or console (local only)
//	OTEL_EXPORTER_OTLP_ENDPOINT     collector, for example http://otel-collector:4317. Unset = no export.
//
// Build with the release data:
//
//	go build -ldflags "-X main.version=0.1.0 -X main.commit=$(git rev-parse HEAD)" ./examples/service
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"

	"github.com/VelimirMueller/synthwerk-contracts/go/examples/service/app"
	"github.com/VelimirMueller/synthwerk-contracts/go/kit/env"
	kitlog "github.com/VelimirMueller/synthwerk-contracts/go/kit/log"
	kitotel "github.com/VelimirMueller/synthwerk-contracts/go/kit/otel"
)

// Set by -ldflags at build time.
var (
	version = "0.0.0-dev"
	commit  = "0000000"
)

func main() {
	if err := start(); err != nil {
		slog.Error("service stopped", slog.String("event.name", "service.failed"), slog.String("exception.message", err.Error()))
		os.Exit(1)
	}
}

func start() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx)
}

func run(ctx context.Context) error {
	// 1. Logger without the OTel bridge, for the setup lines.
	logOpts, err := kitlog.OptionsFromEnv(app.Service, version)
	if err != nil {
		return err
	}
	logger, err := kitlog.New(logOpts)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

	// 2. Telemetry, then the final logger that also writes to OTLP.
	tel, err := kitotel.Setup(ctx, kitotel.Config{ServiceName: logOpts.ServiceName, Version: version, Env: logOpts.Env}, logger)
	if err != nil {
		return err
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(sctx)
	}()
	logOpts.LoggerProvider = tel.LoggerProvider
	if logger, err = kitlog.New(logOpts); err != nil {
		return err
	}
	slog.SetDefault(logger)

	// 3. Dependencies.
	dbURL := env.Get(app.Service, "DATABASE_URL", "")
	if dbURL == "" {
		return errors.New(env.Key(app.Service, "DATABASE_URL") + " is not set")
	}
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	nc, err := nats.Connect(env.Get(app.Service, "NATS_URL", nats.DefaultURL), nats.Name("synthwerk-"+app.Service))
	if err != nil {
		return err
	}
	defer nc.Close()

	a, err := app.New(db, nc, logger, version, commit, logOpts.Env)
	if err != nil {
		return err
	}
	if err := a.Migrate(ctx); err != nil {
		return err
	}
	if logOpts.Env == "local" || logOpts.Env == "ci" {
		if err := app.EnsureStream(ctx, a.JS); err != nil {
			return err
		}
	}

	// 4. HTTP server and relay. /readyz answers 503 until here.
	srv := &http.Server{
		Addr:              env.Get(app.Service, "HTTP_ADDR", ":8080"),
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	a.Health.MarkStarted()
	logger.InfoContext(ctx, "service started", slog.String("event.name", "service.started"), slog.String("server.address", srv.Addr))

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return a.Relay.Run(gctx) })
	g.Go(func() error {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		sctx, cancel := context.WithTimeout(context.WithoutCancel(gctx), 10*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	})
	return g.Wait()
}
