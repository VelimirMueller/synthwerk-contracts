package contract_test

// loadSpec, toJSON, serveAs (llm_test.go) and strict (health_test.go) are in this package.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VelimirMueller/synthwerk-contracts/go/vision"
)

const visionSpec = "../../openapi/vision/openapi.yaml"

func TestVisionContract(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, visionSpec)
	labels := spec.Paths["/v1/orgs/{orgId}/labels"]
	appJSON := "application/json"

	t.Run("LabelSet - spec example - decodes strictly", func(t *testing.T) {
		var set vision.LabelSet
		strict(t, toJSON(t, labels.Get.Responses["200"].Content[appJSON].Example), &set)
		if !set.Custom || len(set.Labels) != 1 || len(set.Topics) == 0 || set.TotalLabels < 1 {
			t.Fatalf("got %+v", set)
		}
		if set.Topics[0].Parent != nil || set.Topics[0].Depth != 0 {
			t.Fatalf("root topic: got %+v", set.Topics[0])
		}
	})

	t.Run("Label and Feedback - spec examples - decode strictly", func(t *testing.T) {
		strict(t, toJSON(t, labels.Post.Responses["201"].Content[appJSON].Example), &vision.Label{})
		feedback := spec.Paths["/v1/orgs/{orgId}/feedback"].Post.Responses["201"].Content[appJSON].Example
		strict(t, toJSON(t, feedback), &vision.Feedback{})
	})

	t.Run("Error - every response example - decodes to a string or a list detail", func(t *testing.T) {
		for name, res := range spec.Components.Responses {
			var e vision.Error
			strict(t, toJSON(t, res.Content[appJSON].Example), &e)
			if _, err := e.Detail.AsErrorDetail0(); err != nil {
				if _, err := e.Detail.AsErrorDetail1(); err != nil {
					t.Fatalf("%s: detail is neither text nor a list", name)
				}
			}
		}
	})

	t.Run("GetLabelSet - client - sends the service bearer to the org path", func(t *testing.T) {
		body := toJSON(t, labels.Get.Responses["200"].Content[appJSON].Example)
		var gotPath, gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
			w.Header().Set("Content-Type", appJSON)
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		client, err := vision.NewClientWithResponses(srv.URL, vision.WithRequestEditorFn(
			func(_ context.Context, r *http.Request) error {
				r.Header.Set("Authorization", "Bearer service-token")
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.VisionGetLabelSetWithResponse(ctx, "283746192837461928")
		if err != nil {
			t.Fatal(err)
		}
		if gotPath != "/v1/orgs/283746192837461928/labels" || gotAuth != "Bearer service-token" {
			t.Fatalf("path %q auth %q", gotPath, gotAuth)
		}
		if res.JSON200 == nil || res.JSON200.OrgId != "283746192837461928" {
			t.Fatalf("got %+v", res.JSON200)
		}
	})

	t.Run("CreateLabel - 409 - typed error body", func(t *testing.T) {
		body := toJSON(t, spec.Components.Responses["Conflict"].Content[appJSON].Example)
		client, err := vision.NewClientWithResponses(serveAs(t, http.StatusConflict, appJSON, body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.VisionCreateLabelWithResponse(ctx, "org-1", vision.CreateLabelRequest{Name: "hut", Topic: "landscape"})
		if err != nil {
			t.Fatal(err)
		}
		if res.JSON409 == nil {
			t.Fatalf("got status %d", res.StatusCode())
		}
	})

	t.Run("DeleteLabel - 204 - no body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			if r.Method != http.MethodDelete || r.URL.Path != "/v1/orgs/org-1/labels/runtime:hut" {
				w.WriteHeader(http.StatusTeapot)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(srv.Close)
		client, err := vision.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.VisionDeleteLabelWithResponse(ctx, "org-1", "runtime:hut")
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode() != http.StatusNoContent {
			t.Fatalf("got status %d", res.StatusCode())
		}
	})
}
