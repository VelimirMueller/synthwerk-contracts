// Package health serves GET /healthz and GET /readyz in the shape of
// openapi/_shared/health.yaml. The body types come from the generated package
// github.com/VelimirMueller/synthwerk-contracts/go/health, so the contract and the
// server cannot drift.
//
//   - /healthz answers 200 {"status":"ok"} at once. It runs no checks.
//   - /readyz answers 503 "down" until MarkStarted. After that it runs every check in
//     parallel, each with its own timeout. The overall status is the worst check status:
//     ok and degraded answer 200, down answers 503.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	api "github.com/VelimirMueller/synthwerk-contracts/go/health"
)

// DefaultTimeout is the timeout of a check that sets none.
const DefaultTimeout = 300 * time.Millisecond

// Budget is the max time /readyz spends on all checks. The contract allows < 1 s.
const Budget = 900 * time.Millisecond

const (
	maxChecks    = 32
	maxDetailLen = 256
	maxNameLen   = 64
	maxVersion   = 64
	maxLatencyMs = 60000
)

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	commitRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

// Check is one readiness check, for example one dependency.
type Check struct {
	// Name matches ^[a-z][a-z0-9_-]*$, for example "postgres".
	Name string
	// Critical: a failure makes the service "down" (503). Else it is "degraded" (200).
	Critical bool
	// Timeout of this check. Zero means DefaultTimeout. Capped by Budget.
	Timeout time.Duration
	// Run returns nil when the dependency works. The error text is never sent:
	// /readyz shows Detail (or "check failed") instead.
	Run func(ctx context.Context) error
	// Detail is the constant text /readyz shows when the check fails. Max 256 characters.
	Detail string
}

// Handler serves both probes.
type Handler struct {
	version string
	commit  string
	checks  []Check
	started atomic.Bool
}

// New returns a handler. version is the release version (max 64 characters), commit
// the git SHA (7 to 40 lowercase hex characters).
func New(version, commit string, checks ...Check) (*Handler, error) {
	var errs []error
	if version == "" || len(version) > maxVersion {
		errs = append(errs, fmt.Errorf("version must have 1 to %d characters", maxVersion))
	}
	if !commitRe.MatchString(commit) {
		errs = append(errs, fmt.Errorf("commit %q is not 7 to 40 lowercase hex characters", commit))
	}
	if len(checks) > maxChecks {
		errs = append(errs, fmt.Errorf("%d checks, max %d", len(checks), maxChecks))
	}
	seen := map[string]bool{}
	for _, c := range checks {
		if !nameRe.MatchString(c.Name) || len(c.Name) > maxNameLen {
			errs = append(errs, fmt.Errorf("check name %q does not match ^[a-z][a-z0-9_-]*$ (max %d)", c.Name, maxNameLen))
		}
		if seen[c.Name] {
			errs = append(errs, fmt.Errorf("check name %q is used twice", c.Name))
		}
		seen[c.Name] = true
		if c.Run == nil {
			errs = append(errs, fmt.Errorf("check %q has no Run func", c.Name))
		}
		if len(c.Detail) > maxDetailLen {
			errs = append(errs, fmt.Errorf("check %q: detail is longer than %d characters", c.Name, maxDetailLen))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("health: %w", errors.Join(errs...))
	}
	return &Handler{version: version, commit: commit, checks: checks}, nil
}

// MarkStarted ends the startup phase. Before it, /readyz answers 503 "down".
func (h *Handler) MarkStarted() { h.started.Store(true) }

// Register adds GET /healthz and GET /readyz to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.Liveness)
	mux.HandleFunc("GET /readyz", h.Readiness)
}

// Liveness serves GET /healthz.
func (h *Handler) Liveness(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, api.Liveness{Status: api.LivenessStatusOk})
}

// Readiness serves GET /readyz.
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	body := h.Check(r.Context())
	code := http.StatusOK
	if body.Status == api.ReadinessStatusDown {
		code = http.StatusServiceUnavailable
	}
	write(w, code, body)
}

// Check runs the checks and returns the /readyz body.
func (h *Handler) Check(ctx context.Context) api.Readiness {
	body := api.Readiness{Version: h.version, Commit: h.commit, Checks: []api.Check{}}
	if !h.started.Load() {
		detail := "startup not complete"
		body.Status = api.ReadinessStatusDown
		body.Checks = append(body.Checks, api.Check{Name: "startup", Status: api.ReadinessStatusDown, Detail: &detail})
		return body
	}

	ctx, cancel := context.WithTimeout(ctx, Budget)
	defer cancel()
	results := make([]api.Check, len(h.checks))
	var wg sync.WaitGroup
	for i, c := range h.checks {
		wg.Go(func() { results[i] = run(ctx, c) })
	}
	wg.Wait()

	body.Status = api.ReadinessStatusOk
	for _, res := range results {
		body.Status = worst(body.Status, res.Status)
	}
	body.Checks = results
	return body
}

func run(ctx context.Context, c Check) api.Check {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- fmt.Errorf("check panicked: %v", p)
			}
		}()
		done <- c.Run(ctx)
	}()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	res := api.Check{Name: c.Name, Status: api.ReadinessStatusOk, LatencyMs: latency(time.Since(start))}
	if err != nil {
		res.Status = api.ReadinessStatusDegraded
		if c.Critical {
			res.Status = api.ReadinessStatusDown
		}
		detail := c.Detail
		if detail == "" {
			detail = "check failed"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			detail = "timeout"
		}
		res.Detail = &detail
	}
	return res
}

func latency(d time.Duration) int {
	ms := d.Milliseconds()
	if ms > maxLatencyMs {
		return maxLatencyMs
	}
	return int(ms)
}

var rank = map[api.ReadinessStatus]int{
	api.ReadinessStatusOk:       0,
	api.ReadinessStatusDegraded: 1,
	api.ReadinessStatusDown:     2,
}

func worst(a, b api.ReadinessStatus) api.ReadinessStatus {
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func write(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
