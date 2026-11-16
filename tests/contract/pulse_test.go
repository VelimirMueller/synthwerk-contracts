package contract_test

// loadSpec, toJSON (llm_test.go) and strict (health_test.go) are in this package.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/VelimirMueller/synthwerk-contracts/go/pulse"
)

const pulseSpec = "../../openapi/pulse/openapi.yaml"

func TestPulseContract(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, pulseSpec)
	appJSON := "application/json"
	example := func(path, status string) []byte {
		t.Helper()
		return toJSON(t, spec.Paths[path].Get.Responses[status].Content[appJSON].Example)
	}

	t.Run("Snapshot - spec example - decodes strictly, checked_at null before the first check", func(t *testing.T) {
		var snap pulse.Snapshot
		strict(t, example("/v1/health", "200"), &snap)
		if len(snap.Targets) != 2 || snap.Targets[0].CheckedAt == nil || snap.Targets[1].CheckedAt != nil {
			t.Fatalf("got %+v", snap)
		}
	})

	t.Run("StatusStreamEvent - snapshot and status examples - decode by the event name", func(t *testing.T) {
		examples := spec.Paths["/v1/health/stream"].Get.Responses["200"].Content["text/event-stream"].Examples
		for name, want := range map[string]string{"snapshot": "snapshot", "status": "status"} {
			var ev pulse.StatusStreamEvent
			strict(t, toJSON(t, examples[name].Value), &ev)
			got, err := ev.Discriminator()
			if err != nil || got != want {
				t.Fatalf("%s: discriminator %q, %v", name, got, err)
			}
		}
	})

	t.Run("PlatformMetrics - spec example - decodes strictly with 7 cost days", func(t *testing.T) {
		var m pulse.PlatformMetrics
		strict(t, example("/v1/metrics", "200"), &m)
		if m.AiCost == nil || len(m.AiCost.Days) != 7 || m.AiCost.Currency != "USD" || m.Services == nil {
			t.Fatalf("got %+v", m)
		}
		if last := m.AiCost.Days[6]; last.Date.String() != "2026-11-16" || last.UnpricedAnswers != 2 {
			t.Fatalf("today: got %+v", last)
		}
	})

	t.Run("Alerts - spec example - critical first, trace link only with a trace id", func(t *testing.T) {
		var list pulse.AlertList
		strict(t, example("/v1/alerts", "200"), &list)
		if len(list.Alerts) != 2 || list.Alerts[0].Severity != pulse.AlertSeverityCritical {
			t.Fatalf("got %+v", list)
		}
		down, errs := list.Alerts[0], list.Alerts[1]
		if down.Rule != pulse.AlertRuleTargetDown || down.Value != nil || down.TraceId != nil || down.TraceUrl != nil {
			t.Fatalf("target_down: got %+v", down)
		}
		traceID := regexp.MustCompile(`^[0-9a-f]{32}$`)
		if errs.Rule != pulse.AlertRuleServiceErrors || errs.TraceId == nil || !traceID.MatchString(*errs.TraceId) ||
			errs.TraceUrl == nil || !regexp.MustCompile(`^https?://`).MatchString(*errs.TraceUrl) {
			t.Fatalf("service_errors: got %+v", errs)
		}
	})

	t.Run("Problem - every response example - decodes strictly with about:blank", func(t *testing.T) {
		for name, res := range spec.Components.Responses {
			var p pulse.Problem
			strict(t, toJSON(t, res.Content["application/problem+json"].Example), &p)
			if p.Type != "about:blank" || p.Code == "" {
				t.Fatalf("%s: got %+v", name, p)
			}
		}
	})

	t.Run("GetOrgMetrics - client - sends the bearer to the org path", func(t *testing.T) {
		body := example("/v1/orgs/{orgId}/metrics", "200")
		var gotPath, gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
			w.Header().Set("Content-Type", appJSON)
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		client, err := pulse.NewClientWithResponses(srv.URL, pulse.WithRequestEditorFn(
			func(_ context.Context, r *http.Request) error {
				r.Header.Set("Authorization", "Bearer user-token")
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.PulseGetOrgMetricsWithResponse(ctx, "339201")
		if err != nil {
			t.Fatal(err)
		}
		if gotPath != "/v1/orgs/339201/metrics" || gotAuth != "Bearer user-token" {
			t.Fatalf("path %q auth %q", gotPath, gotAuth)
		}
		if res.JSON200 == nil || res.JSON200.OrgId != "339201" || res.JSON200.AiCost == nil {
			t.Fatalf("got %+v", res.JSON200)
		}
	})

	t.Run("ListAlerts - 403 - typed problem body", func(t *testing.T) {
		body := toJSON(t, spec.Components.Responses["Forbidden"].Content["application/problem+json"].Example)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		client, err := pulse.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.PulseListAlertsWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode() != http.StatusForbidden || res.ApplicationproblemJSON403 == nil || res.ApplicationproblemJSON403.Code != "forbidden" {
			t.Fatalf("status %d body %+v", res.StatusCode(), res.ApplicationproblemJSON403)
		}
	})
	t.Run("PublicStats - spec example - decodes strictly, aggregates only", func(t *testing.T) {
		var st pulse.PublicStats
		strict(t, example("/v1/public-stats", "200"), &st)
		if st.WindowDays != 30 || st.UptimeRatio == nil || *st.UptimeRatio > 1 || st.P95Ms == nil || st.AnsweredChats == nil {
			t.Fatalf("got %+v", st)
		}
	})

	t.Run("GetPublicStats - client - calls the path without a token", func(t *testing.T) {
		body := example("/v1/public-stats", "200")
		var gotPath, gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
			w.Header().Set("Content-Type", appJSON)
			w.Header().Set("Cache-Control", "public, max-age=60")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		client, err := pulse.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.PulseGetPublicStatsWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if gotPath != "/v1/public-stats" || gotAuth != "" {
			t.Fatalf("path %q auth %q", gotPath, gotAuth)
		}
		if res.JSON200 == nil || res.JSON200.AnsweredChats == nil || *res.JSON200.AnsweredChats != 1234 {
			t.Fatalf("got %+v", res.JSON200)
		}
	})
}
