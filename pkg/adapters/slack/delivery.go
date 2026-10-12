package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type DeliveryResult struct {
	State     string     `json:"state"`
	ChannelID string     `json:"channel_id,omitempty"`
	MessageID string     `json:"message_id,omitempty"`
	ThreadID  string     `json:"thread_id,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	RetryAt   *time.Time `json:"retry_at,omitempty"`
}
type SendRequest struct {
	Token        string `json:"-"`
	ConnectionID string
	OperationID  string
	TeamID       string
	ChannelID    string
	ThreadID     string
	Text         string
	Attachments  []map[string]any
}
type Sender interface {
	SendSlack(context.Context, SendRequest) (DeliveryResult, error)
}
type SenderFunc func(context.Context, SendRequest) (DeliveryResult, error)

func (f SenderFunc) SendSlack(ctx context.Context, r SendRequest) (DeliveryResult, error) {
	return f(ctx, r)
}
func WithSender(sender Sender) Option { return func(a *Adapter) { a.sender = sender } }
func (a *Adapter) SendWithResult(ctx context.Context, msg adapters.Message) (DeliveryResult, error) {
	input := SendRequest{Token: strings.TrimSpace(firstNonEmpty(stringValue(msg.Metadata, "token"), secretString(msg.Metadata, "token"), secretString(msg.Metadata, "default"), a.cfg.Token)), ConnectionID: stringValue(msg.Metadata, "connection_id"), OperationID: stringValue(msg.Metadata, "operation_id"), TeamID: stringValue(msg.Metadata, "team_id"), ChannelID: firstNonEmpty(stringValue(msg.Metadata, "channel"), a.cfg.Channel), ThreadID: stringValue(msg.Metadata, "thread_ts"), Text: firstNonEmpty(stringValue(msg.Metadata, "body"), msg.Body), Attachments: slackAttachments(msg.Attachments)}
	if html := stringValue(msg.Metadata, "html_body"); html != "" {
		input.Text = stripHTML(html)
	}
	invalid := DeliveryResult{State: "unsent", Reason: "invalid_input"}
	if input.ChannelID == "" || strings.TrimSpace(input.Text) == "" {
		return invalid, errors.New("slack: channel and body required")
	}
	if a.cfg.DryRun {
		return DeliveryResult{State: "dry_run"}, nil
	}
	var result DeliveryResult
	var err error
	if a.sender != nil {
		result, err = a.sender.SendSlack(ctx, input)
	} else {
		result, err = a.sendHTTP(ctx, input)
	}
	if err != nil {
		return result, err
	}
	if result.State == "sent" {
		if (result.ChannelID != input.ChannelID && !strings.HasPrefix(input.ChannelID, "#")) || result.ChannelID == "" || result.MessageID == "" || result.ThreadID == "" || input.ThreadID != "" && result.ThreadID != input.ThreadID {
			return DeliveryResult{State: "unknown", Reason: "receipt_mismatch"}, errors.New("slack: receipt_mismatch")
		}
		a.base.LogSuccess(a.name, msg)
		return result, nil
	}
	if result.State == "dry_run" {
		return result, errors.New("slack: unexpected dry run")
	}
	return result, errors.New("slack: " + safeDeliveryReason(result.Reason))
}
func safeDeliveryReason(code string) string {
	switch code {
	case "rate_limited", "credential_required", "request_not_sent", "outcome_unknown", "receipt_mismatch", "provider_rejected", "invalid_input", "grant_missing", "credential_unavailable", "installation_unavailable", "selection_required":
		return code
	}
	return "delivery_failed"
}
func (a *Adapter) sendHTTP(ctx context.Context, input SendRequest) (DeliveryResult, error) {
	result := DeliveryResult{State: "unsent", Reason: "credential_required"}
	if input.Token == "" {
		return result, nil
	}
	payload := map[string]any{"channel": input.ChannelID, "text": input.Text, "mrkdwn": true}
	if input.ThreadID != "" {
		payload["thread_ts"] = input.ThreadID
	}
	if len(input.Attachments) > 0 {
		payload["attachments"] = input.Attachments
	}
	body, err := json.Marshal(payload)
	if err != nil {
		result.Reason = "invalid_input"
		return result, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.cfg.BaseURL, "/")+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		result.Reason = "invalid_input"
		return result, nil
	}
	req.Header.Set("Authorization", "Bearer "+input.Token)
	req.Header.Set("Content-Type", "application/json")
	if ctx.Err() != nil {
		result.Reason = "request_not_sent"
		return result, nil
	}
	response, err := a.client.Do(req)
	if err != nil || response == nil || response.Body == nil {
		return DeliveryResult{State: "unknown", Reason: "outcome_unknown"}, nil
	}
	defer response.Body.Close()
	if response.StatusCode == 429 {
		seconds, err := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 64)
		if err != nil || seconds <= 0 || seconds > 86400 {
			seconds = 60
		}
		at := time.Now().UTC().Add(time.Duration(seconds) * time.Second)
		return DeliveryResult{State: "rate_limited", Reason: "rate_limited", RetryAt: &at}, nil
	}
	if response.StatusCode >= 500 || response.StatusCode >= 300 && response.StatusCode < 400 {
		return DeliveryResult{State: "unknown", Reason: "outcome_unknown"}, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Reason = "provider_rejected"
		return result, nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return DeliveryResult{State: "unknown", Reason: "outcome_unknown"}, nil
	}
	var value struct {
		OK                 bool
		Channel, TS, Error string
		Message            struct {
			Thread string `json:"thread_ts"`
		}
	}
	if json.Unmarshal(data, &value) != nil {
		return DeliveryResult{State: "unknown", Reason: "outcome_unknown"}, nil
	}
	if !value.OK {
		if value.Error == "ratelimited" || value.Error == "rate_limited" {
			at := time.Now().UTC().Add(time.Minute)
			return DeliveryResult{State: "rate_limited", Reason: "rate_limited", RetryAt: &at}, nil
		}
		result.Reason = "provider_rejected"
		return result, nil
	}
	thread := value.Message.Thread
	if input.ThreadID == "" {
		thread = value.TS
	}
	return DeliveryResult{State: "sent", ChannelID: value.Channel, MessageID: value.TS, ThreadID: thread}, nil
}
