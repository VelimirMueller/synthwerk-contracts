// Package cloudevents builds Synthwerk CloudEvents 1.0.2 in JSON format.
//
// The shape follows events/envelope.schema.json, the names follow docs/naming.md.
// New gives every event a UUIDv7 id. The outbox relay sends the id as Nats-Msg-Id,
// so JetStream drops a second publish of the same event.
package cloudevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
)

// SpecVersion is the CloudEvents spec version. 1.0.2 is a patch of 1.0.
const SpecVersion = "1.0"

// ContentType is the datacontenttype of every Synthwerk event.
const ContentType = "application/json"

// typePrefix starts every event type. The NATS subject replaces it with "evt.".
const typePrefix = "dev.synthwerk."

var (
	typeRe   = regexp.MustCompile(`^dev\.synthwerk\.[a-z][a-z0-9]*\.[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\.v[1-9][0-9]*$`)
	sourceRe = regexp.MustCompile(`^/synthwerk/[a-z][a-z0-9-]*/(local|dev|stg|prd)$`)
	extIDRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	actorRe  = regexp.MustCompile(`^(user:[A-Za-z0-9_-]{1,128}|service:[a-z][a-z0-9-]{0,62}|system)$`)
	parentRe = regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)
)

// Event is one CloudEvent in JSON format (events/envelope.schema.json).
type Event struct {
	SpecVersion     string    `json:"specversion"`
	ID              string    `json:"id"`
	Source          string    `json:"source"`
	Type            string    `json:"type"`
	Subject         string    `json:"subject,omitempty"`
	Time            time.Time `json:"time"`
	DataContentType string    `json:"datacontenttype"`
	DataSchema      string    `json:"dataschema,omitempty"`
	OrgID           string    `json:"orgid,omitempty"`
	// TenantID is the tenantid extension.
	//
	// Deprecated: use OrgID / WithOrg (D-44).
	TenantID    string          `json:"tenantid,omitempty"`
	TraceParent string          `json:"traceparent,omitempty"`
	Actor       string          `json:"actor,omitempty"`
	Data        json.RawMessage `json:"data"`
}

// Option sets an optional attribute on a new event.
type Option func(*Event)

// WithSubject sets the CloudEvents subject: the id of the main entity.
func WithSubject(subject string) Option { return func(e *Event) { e.Subject = subject } }

// WithOrg sets the orgid extension: the org the fact belongs to (D-44).
func WithOrg(orgID string) Option { return func(e *Event) { e.OrgID = orgID } }

// WithTenant sets the tenantid extension.
//
// Deprecated: use OrgID / WithOrg (D-44).
func WithTenant(tenantID string) Option { return func(e *Event) { e.TenantID = tenantID } }

// WithActor sets the actor extension: user:<sub>, service:<name> or system.
func WithActor(actor string) Option { return func(e *Event) { e.Actor = actor } }

// WithDataSchema sets the $id of the data schema.
func WithDataSchema(uri string) Option { return func(e *Event) { e.DataSchema = uri } }

// WithTime sets the time of the fact. The default is now. The value is stored in UTC.
func WithTime(t time.Time) Option { return func(e *Event) { e.Time = t } }

// New builds a valid event. The traceparent comes from the span in ctx, if there is one.
// data must marshal to a JSON object.
func New(ctx context.Context, eventType, source string, data any, opts ...Option) (Event, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Event{}, fmt.Errorf("cloudevents: new id: %w", err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return Event{}, fmt.Errorf("cloudevents: marshal data: %w", err)
	}
	e := Event{
		SpecVersion:     SpecVersion,
		ID:              id.String(),
		Source:          source,
		Type:            eventType,
		Time:            time.Now(),
		DataContentType: ContentType,
		TraceParent:     traceParent(ctx),
		Data:            raw,
	}
	for _, opt := range opts {
		opt(&e)
	}
	e.Time = e.Time.UTC()
	if err := e.Validate(); err != nil {
		return Event{}, err
	}
	return e, nil
}

// Validate checks the envelope rules that do not need the data schema.
func (e Event) Validate() error {
	var errs []error
	check := func(ok bool, format string, args ...any) {
		if !ok {
			errs = append(errs, fmt.Errorf(format, args...))
		}
	}
	id, idErr := uuid.Parse(e.ID)
	check(e.SpecVersion == SpecVersion, "specversion %q is not %q", e.SpecVersion, SpecVersion)
	check(idErr == nil && id.Version() == 7 && id.String() == e.ID, "id %q is not a lowercase UUIDv7", e.ID)
	check(sourceRe.MatchString(e.Source) && len(e.Source) <= 128, "source %q does not match /synthwerk/<service>/<env>", e.Source)
	check(typeRe.MatchString(e.Type) && len(e.Type) <= 128, "type %q does not match dev.synthwerk.<domain>.<entity>.<verb>.v<N>", e.Type)
	check(len(e.Subject) <= 128, "subject is longer than 128 characters")
	check(!e.Time.IsZero(), "time is missing")
	check(e.DataContentType == ContentType, "datacontenttype %q is not %q", e.DataContentType, ContentType)
	check(len(e.DataSchema) <= 256, "dataschema is longer than 256 characters")
	check(e.OrgID == "" || extIDRe.MatchString(e.OrgID), "orgid %q has characters outside [A-Za-z0-9_-] or is too long", e.OrgID)
	check(e.TenantID == "" || extIDRe.MatchString(e.TenantID), "tenantid %q has characters outside [A-Za-z0-9_-] or is too long", e.TenantID)
	check(e.TraceParent == "" || parentRe.MatchString(e.TraceParent), "traceparent %q is not W3C version 00", e.TraceParent)
	check(e.Actor == "" || actorRe.MatchString(e.Actor), "actor %q is not user:<sub>, service:<name> or system", e.Actor)
	check(isObject(e.Data), "data is not a JSON object")
	if len(errs) > 0 {
		return fmt.Errorf("cloudevents: invalid event: %w", errors.Join(errs...))
	}
	return nil
}

// NATSSubject returns the NATS subject of an event type: "evt." + the type without "dev.synthwerk.".
func NATSSubject(eventType string) (string, error) {
	if !typeRe.MatchString(eventType) {
		return "", fmt.Errorf("cloudevents: type %q does not match dev.synthwerk.<domain>.<entity>.<verb>.v<N>", eventType)
	}
	return "evt." + strings.TrimPrefix(eventType, typePrefix), nil
}

// Source returns the source attribute /synthwerk/<service>/<env>.
// The envelope knows no "ci" environment: CI runs publish as "local".
func Source(service, env string) (string, error) {
	if env == "ci" {
		env = "local"
	}
	s := "/synthwerk/" + service + "/" + env
	if !sourceRe.MatchString(s) {
		return "", fmt.Errorf("cloudevents: source %q does not match /synthwerk/<service>/<local|dev|stg|prd>", s)
	}
	return s, nil
}

func traceParent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

func isObject(raw json.RawMessage) bool {
	var v map[string]json.RawMessage
	return json.Unmarshal(raw, &v) == nil && v != nil
}
