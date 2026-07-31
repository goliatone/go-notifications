package receipts

import "github.com/google/uuid"

// Status describes the aggregate state of notification intake or delivery.
type Status string

const (
	StatusAccepted    Status = "accepted"
	StatusScheduled   Status = "scheduled"
	StatusDispatching Status = "dispatching"
	StatusRetrying    Status = "retrying"
	StatusProcessed   Status = "processed"
	StatusPartial     Status = "partial"
	StatusFailed      Status = "failed"
	StatusReplayed    Status = "replayed"
)

// OutcomeStatus describes one recipient/channel/provider delivery.
type OutcomeStatus string

const (
	OutcomeDelivered OutcomeStatus = "delivered"
	OutcomeFailed    OutcomeStatus = "failed"
	OutcomeSkipped   OutcomeStatus = "skipped"
	OutcomePending   OutcomeStatus = "pending"
)

// DeliveryOutcome is deliberately free of destinations, content, URLs, and
// provider response bodies.
type DeliveryOutcome struct {
	MessageID    uuid.UUID     `json:"message_id,omitempty"`
	SubjectID    string        `json:"subject_id,omitempty"`
	Channel      string        `json:"channel"`
	Provider     string        `json:"provider,omitempty"`
	Status       OutcomeStatus `json:"status"`
	AttemptCount int           `json:"attempt_count,omitempty"`
	ErrorCode    string        `json:"error_code,omitempty"`
}

// DispatchReceipt is the durable, privacy-safe result of intake or delivery.
type DispatchReceipt struct {
	EventID          uuid.UUID         `json:"event_id"`
	PublicationID    uuid.UUID         `json:"publication_id,omitempty"`
	RetryOperationID uuid.UUID         `json:"retry_operation_id,omitempty"`
	Status           Status            `json:"status"`
	Replay           bool              `json:"replay"`
	CorrelationID    string            `json:"correlation_id,omitempty"`
	RequestID        string            `json:"request_id,omitempty"`
	Outcomes         []DeliveryOutcome `json:"outcomes,omitempty"`
}
