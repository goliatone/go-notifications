package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// RecordMeta captures identifiers and audit fields shared across entities.
type RecordMeta struct {
	ID        uuid.UUID `bun:",pk,type:uuid" json:"id"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	DeletedAt time.Time `bun:",soft_delete,nullzero" json:"deleted_at,omitempty"`
}

// EnsureID assigns a UUID when the struct is about to be persisted.
func (m *RecordMeta) EnsureID() {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
}

// JSONMap persists arbitrary metadata fields as JSON.
type JSONMap map[string]any

// Value implements driver.Valuer.
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner.
func (m *JSONMap) Scan(value any) error {
	if m == nil {
		return errors.New("JSONMap: Scan on nil pointer")
	}
	switch v := value.(type) {
	case nil:
		*m = nil
		return nil
	case []byte:
		return json.Unmarshal(v, m)
	case string:
		return json.Unmarshal([]byte(v), m)
	default:
		return fmt.Errorf("JSONMap: unsupported type %T", value)
	}
}

// StringList stores []string as JSON.
type StringList []string

func (s StringList) Value() (driver.Value, error) {
	return json.Marshal([]string(s))
}

func (s *StringList) Scan(value any) error {
	if s == nil {
		return errors.New("StringList: Scan on nil pointer")
	}
	switch v := value.(type) {
	case nil:
		*s = nil
		return nil
	case []byte:
		return json.Unmarshal(v, (*[]string)(s))
	case string:
		return json.Unmarshal([]byte(v), (*[]string)(s))
	default:
		return fmt.Errorf("StringList: unsupported type %T", value)
	}
}

// TemplateSource describes where a template body originated.
type TemplateSource struct {
	Type      string  `json:"type"`
	Reference string  `json:"reference"`
	Payload   JSONMap `json:"payload"`
}

func (s TemplateSource) Value() (driver.Value, error) {
	if s.Type == "" && s.Reference == "" && len(s.Payload) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(s)
}

func (s *TemplateSource) Scan(value any) error {
	if s == nil {
		return errors.New("TemplateSource: Scan on nil pointer")
	}
	switch v := value.(type) {
	case nil:
		*s = TemplateSource{}
		return nil
	case []byte:
		if len(v) == 0 {
			*s = TemplateSource{}
			return nil
		}
		return json.Unmarshal(v, s)
	case string:
		if v == "" {
			*s = TemplateSource{}
			return nil
		}
		return json.Unmarshal([]byte(v), s)
	default:
		return fmt.Errorf("TemplateSource: unsupported type %T", value)
	}
}

// TemplateSchema tracks required and optional placeholders.
type TemplateSchema struct {
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

// IsZero reports whether any constraints are defined.
func (s TemplateSchema) IsZero() bool {
	return len(s.Required) == 0 && len(s.Optional) == 0
}

func (s TemplateSchema) Value() (driver.Value, error) {
	if len(s.Required) == 0 && len(s.Optional) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(s)
}

func (s *TemplateSchema) Scan(value any) error {
	if s == nil {
		return errors.New("TemplateSchema: Scan on nil pointer")
	}
	switch v := value.(type) {
	case nil:
		*s = TemplateSchema{}
		return nil
	case []byte:
		if len(v) == 0 {
			*s = TemplateSchema{}
			return nil
		}
		return json.Unmarshal(v, s)
	case string:
		if v == "" {
			*s = TemplateSchema{}
			return nil
		}
		return json.Unmarshal([]byte(v), s)
	default:
		return fmt.Errorf("TemplateSchema: unsupported type %T", value)
	}
}

// NotificationDefinition describes a notification type.
type NotificationDefinition struct {
	bun.BaseModel `bun:"table:notification_definitions"`
	RecordMeta

	Code        string     `bun:",unique,nullzero,notnull"`
	Name        string     `bun:",nullzero,notnull"`
	Description string     `bun:",nullzero"`
	Severity    string     `bun:",nullzero"`
	Category    string     `bun:",nullzero"`
	Channels    StringList `bun:"type:jsonb,nullzero"`
	Metadata    JSONMap    `bun:"type:jsonb,nullzero"`
	// Template references are logical names, resolved by services.
	TemplateKeys StringList `bun:"type:jsonb,nullzero"`
	// Policy stores throttling/digest requirements.
	Policy JSONMap `bun:"type:jsonb,nullzero"`
}

// NotificationTemplate stores channel-specific template configuration.
type NotificationTemplate struct {
	bun.BaseModel `bun:"table:notification_templates"`
	RecordMeta

	Code        string         `bun:",unique,nullzero,notnull"`
	Channel     string         `bun:",nullzero,notnull"`
	Description string         `bun:",nullzero"`
	Body        string         `bun:",nullzero"`
	Subject     string         `bun:",nullzero"`
	Locale      string         `bun:",nullzero"`
	Format      string         `bun:",nullzero"`
	Revision    int            `bun:",nullzero"`
	Source      TemplateSource `bun:"type:jsonb,nullzero"`
	Schema      TemplateSchema `bun:"type:jsonb,nullzero"`
	Metadata    JSONMap        `bun:"type:jsonb,nullzero"`
}

// NotificationEvent captures input events before fan-out.
type NotificationEvent struct {
	bun.BaseModel `bun:"table:notification_events"`
	RecordMeta

	DefinitionCode string     `bun:",nullzero,notnull"`
	TenantID       string     `bun:",nullzero"`
	ActorID        string     `bun:",nullzero"`
	Recipients     StringList `bun:"type:jsonb,nullzero"`
	Context        JSONMap    `bun:"type:jsonb,nullzero"`
	ScheduledAt    time.Time  `bun:",nullzero"`
	Status         string     `bun:",nullzero"`
}

// NotificationMessage represents a concrete rendered message.
type NotificationMessage struct {
	bun.BaseModel `bun:"table:notification_messages"`
	RecordMeta

	EventID     uuid.UUID `bun:",nullzero,notnull"`
	Channel     string    `bun:",nullzero,notnull"`
	Locale      string    `bun:",nullzero"`
	Subject     string    `bun:",nullzero"`
	Body        string    `bun:",nullzero"`
	ActionURL   string    `bun:",nullzero" json:"action_url"`
	ManifestURL string    `bun:",nullzero" json:"manifest_url"`
	URL         string    `bun:",nullzero" json:"url"`
	Receiver    string    `bun:",nullzero,notnull"`
	Status      string    `bun:",nullzero"`
	Metadata    JSONMap   `bun:"type:jsonb,nullzero"`
}

// DeliveryAttempt tracks adapter executions.
type DeliveryAttempt struct {
	bun.BaseModel `bun:"table:notification_delivery_attempts"`
	RecordMeta

	MessageID uuid.UUID `bun:",nullzero,notnull"`
	Adapter   string    `bun:",nullzero,notnull"`
	Status    string    `bun:",nullzero"`
	Error     string    `bun:",nullzero"`
	Payload   JSONMap   `bun:"type:jsonb,nullzero"`
}

// NotificationPreference captures opt-in/out settings.
type NotificationPreference struct {
	bun.BaseModel `bun:"table:notification_preferences"`
	RecordMeta

	SubjectID       string  `bun:",nullzero,notnull" json:"subject_id"`
	SubjectType     string  `bun:",nullzero,notnull" json:"subject_type"` // user, tenant, group, etc.
	DefinitionCode  string  `bun:",nullzero" json:"definition_code"`
	Channel         string  `bun:",nullzero" json:"channel"`
	Locale          string  `bun:",nullzero" json:"locale"`
	Enabled         bool    `bun:",nullzero" json:"enabled"`
	QuietHours      JSONMap `bun:"type:jsonb,nullzero" json:"quiet_hours,omitempty"`
	AdditionalRules JSONMap `bun:"type:jsonb,nullzero" json:"additional_rules,omitempty"`
}

// SubscriptionGroup represents named cohorts.
type SubscriptionGroup struct {
	bun.BaseModel `bun:"table:notification_subscription_groups"`
	RecordMeta

	Code        string  `bun:",unique,nullzero,notnull"`
	Name        string  `bun:",nullzero,notnull"`
	Description string  `bun:",nullzero"`
	Metadata    JSONMap `bun:"type:jsonb,nullzero"`
}

// InboxItem stores in-app notifications.
type InboxItem struct {
	bun.BaseModel `bun:"table:notification_inbox_items"`
	RecordMeta

	UserID       string    `bun:",nullzero,notnull" json:"user_id"`
	MessageID    uuid.UUID `bun:",nullzero" json:"message_id"`
	Title        string    `bun:",nullzero" json:"title"`
	Body         string    `bun:",nullzero" json:"body"`
	Locale       string    `bun:",nullzero" json:"locale"`
	Unread       bool      `bun:",nullzero" json:"unread"`
	Pinned       bool      `bun:",nullzero" json:"pinned"`
	ActionURL    string    `bun:",nullzero" json:"action_url"`
	Metadata     JSONMap   `bun:"type:jsonb,nullzero" json:"metadata,omitempty"`
	ReadAt       time.Time `bun:",nullzero" json:"read_at,omitempty"`
	DismissedAt  time.Time `bun:",nullzero" json:"dismissed_at,omitempty"`
	SnoozedUntil time.Time `bun:",nullzero" json:"snoozed_until,omitempty"`
}

// Domain constants for statuses.
const (
	EventStatusPending   = "pending"
	EventStatusScheduled = "scheduled"
	EventStatusProcessed = "processed"
	EventStatusFailed    = "failed"

	MessageStatusPending   = "pending"
	MessageStatusDelivered = "delivered"
	MessageStatusFailed    = "failed"

	AttemptStatusPending   = "pending"
	AttemptStatusSucceeded = "succeeded"
	AttemptStatusFailed    = "failed"
)
