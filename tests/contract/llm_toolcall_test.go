package contract_test

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/google/uuid"
)

// TestLlmToolCallV3: the audit fact of an admin AI tool call (synthwerk#53) names the
// identity org twice, so identity can list it: orgid = data.org_id. The subject is the
// conversation, the actor a user, and data holds no tool arguments or results.
func TestLlmToolCallV3(t *testing.T) {
	raw, err := os.ReadFile("../../events/examples/dev.synthwerk.llm.toolcall.executed.v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var ev struct {
		OrgID   string          `json:"orgid"`
		Subject string          `json:"subject"`
		Actor   string          `json:"actor"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	var d struct {
		Tool           string `json:"tool"`
		Scope          string `json:"scope"`
		OrgID          string `json:"org_id"`
		ConversationID string `json:"conversation_id"`
		Outcome        string `json:"outcome"`
		DurationMS     int    `json:"duration_ms"`
	}
	dec := json.NewDecoder(bytes.NewReader(ev.Data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		t.Fatalf("data has a field outside the schema: %v", err)
	}
	t.Run("ToolCall v3 example - org scope - orgid = data.org_id, subject = conversation id, actor user", func(t *testing.T) {
		if _, err := uuid.Parse(d.OrgID); err != nil || ev.OrgID != d.OrgID {
			t.Errorf("orgid %q, data.org_id %q", ev.OrgID, d.OrgID)
		}
		if ev.Subject != d.ConversationID {
			t.Errorf("subject %q, conversation_id %q", ev.Subject, d.ConversationID)
		}
		if !regexp.MustCompile(`^user:[A-Za-z0-9_-]{1,128}$`).MatchString(ev.Actor) {
			t.Errorf("actor %q", ev.Actor)
		}
		if (d.Outcome != "ok" && d.Outcome != "denied" && d.Outcome != "error") || d.DurationMS < 0 {
			t.Errorf("outcome %q duration %d", d.Outcome, d.DurationMS)
		}
		if d.Scope != "org" || !regexp.MustCompile(`^[a-z]+_[a-z]+_[a-z_]+$`).MatchString(d.Tool) {
			t.Errorf("scope %q tool %q", d.Scope, d.Tool)
		}
	})
}
