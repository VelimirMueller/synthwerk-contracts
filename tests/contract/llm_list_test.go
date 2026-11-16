package contract_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"github.com/VelimirMueller/synthwerk-contracts/go/llm"
)

// TestLLMListConversations covers GET /v1/conversations of synthwerk#54: the spec
// example parses strictly and keeps its own rules, and the generated client sends the
// paging parameters.
func TestLLMListConversations(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, "../../openapi/llm/openapi.yaml")
	list := spec.Paths["/v1/conversations"].Get
	body := toJSON(t, list.Responses["200"].Content["application/json"].Example)

	t.Run("ConversationPage - spec example - decodes strictly, newest first, short titles, UUIDv7 ids", func(t *testing.T) {
		var page llm.ConversationPage
		strict(t, body, &page)
		if len(page.Items) < 2 || page.NextCursor == nil {
			t.Fatalf("the example needs two items and a cursor: %+v", page)
		}
		for i, it := range page.Items {
			if n := utf8.RuneCountInString(it.Title); n == 0 || n > 80 {
				t.Errorf("item %d: title has %d characters", i, n)
			}
			if i > 0 && it.CreatedAt.After(page.Items[i-1].CreatedAt) {
				t.Errorf("item %d is newer than item %d", i, i-1)
			}
		}
		for i, it := range page.Items {
			if it.Id.Version() != 7 {
				t.Errorf("item %d: id %s is not a UUIDv7", i, it.Id)
			}
		}
	})

	t.Run("ListConversations - limit and cursor - client sends GET /v1/conversations with both", func(t *testing.T) {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		c, err := llm.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		limit, cursor := 10, "abc"
		res, err := c.LlmListConversationsWithResponse(ctx, &llm.LlmListConversationsParams{Limit: &limit, Cursor: &cursor})
		if err != nil {
			t.Fatal(err)
		}
		if got != "GET /v1/conversations?cursor=abc&limit=10" && got != "GET /v1/conversations?limit=10&cursor=abc" {
			t.Fatalf("request %q", got)
		}
		if res.JSON200 == nil || len(res.JSON200.Items) != 2 {
			t.Fatalf("response %+v", res.JSON200)
		}
	})

	t.Run("ListConversations - bad limit - 400 invalid_request", func(t *testing.T) {
		raw := toJSON(t, spec.Components.Responses["InvalidRequest"].Content["application/problem+json"].Example)
		c, err := llm.NewClientWithResponses(serveAs(t, http.StatusBadRequest, "application/problem+json", raw))
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.LlmListConversationsWithResponse(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.ApplicationproblemJSON400 == nil || res.ApplicationproblemJSON400.Code != "invalid_request" {
			t.Fatalf("400 body %+v", res.ApplicationproblemJSON400)
		}
	})
}
