package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	pkgoptions "github.com/goliatone/go-notifications/pkg/options"
	prefsvc "github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/goliatone/go-notifications/pkg/templates"
	opts "github.com/goliatone/go-options"
)

// Dependencies groups the repositories/services required by the dispatcher.
type inboxDeliverer interface {
	DeliverFromMessage(ctx context.Context, msg *domain.NotificationMessage) error
}

type Dependencies struct {
	Definitions store.NotificationDefinitionRepository
	Events      store.NotificationEventRepository
	Messages    store.NotificationMessageRepository
	Attempts    store.DeliveryAttemptRepository
	Templates   *templates.Service
	Registry    *adapters.Registry
	Attachments adapters.AttachmentResolver
	Logger      logger.Logger
	Config      config.DispatcherConfig
	Preferences *prefsvc.Service
	Inbox       inboxDeliverer
	Secrets     secrets.Resolver
	Activity    activity.Hooks
}

// Service expands events into rendered messages and routes them to adapters.
type Service struct {
	definitions store.NotificationDefinitionRepository
	events      store.NotificationEventRepository
	messages    store.NotificationMessageRepository
	attempts    store.DeliveryAttemptRepository
	templates   *templates.Service
	registry    *adapters.Registry
	attachments adapters.AttachmentResolver
	logger      logger.Logger
	cfg         config.DispatcherConfig
	preferences *prefsvc.Service
	inbox       inboxDeliverer
	secrets     secrets.Resolver
	activity    activity.Hooks
}

// DispatchOptions allow callers to override channels/locales.
type DispatchOptions struct {
	Channels []string
	Locale   string
}

var (
	ErrMissingDefinitions = errors.New("dispatcher: definition repository is required")
	ErrMissingTemplates   = errors.New("dispatcher: templates service is required")
	ErrMissingRegistry    = errors.New("dispatcher: adapter registry is required")
)

// New builds the dispatcher service.
func New(deps Dependencies) (*Service, error) {
	if deps.Definitions == nil {
		return nil, ErrMissingDefinitions
	}

	if deps.Templates == nil {
		return nil, ErrMissingTemplates
	}

	if deps.Registry == nil {
		return nil, ErrMissingRegistry
	}

	if deps.Logger == nil {
		deps.Logger = &logger.Nop{}
	}

	if deps.Config.MaxWorkers <= 0 {
		deps.Config.MaxWorkers = 4
	}

	if deps.Config.MaxRetries <= 0 {
		deps.Config.MaxRetries = 3
	}

	return &Service{
		definitions: deps.Definitions,
		events:      deps.Events,
		messages:    deps.Messages,
		attempts:    deps.Attempts,
		templates:   deps.Templates,
		registry:    deps.Registry,
		attachments: deps.Attachments,
		logger:      deps.Logger,
		cfg:         deps.Config,
		preferences: deps.Preferences,
		inbox:       deps.Inbox,
		secrets:     deps.Secrets,
		activity:    deps.Activity,
	}, nil
}

// Dispatch expands the stored event into deliveries using the configured adapters.
func (s *Service) Dispatch(ctx context.Context, event *domain.NotificationEvent, opts DispatchOptions) error {
	if event == nil {
		return errors.New("dispatcher: event is required")
	}
	definition, err := s.definitions.GetByCode(ctx, event.DefinitionCode)
	if err != nil {
		return fmt.Errorf("dispatcher: load definition: %w", err)
	}

	channels := opts.Channels
	if len(channels) == 0 {
		channels = definition.Channels
	}
	if len(channels) == 0 {
		return errors.New("dispatcher: no channels configured")
	}
	recipients := event.Recipients
	if len(recipients) == 0 {
		return errors.New("dispatcher: event has no recipients")
	}

	jobs := make(chan deliveryJob, len(channels)*len(recipients))
	errCh := make(chan error, len(channels)*len(recipients))
	var wg sync.WaitGroup
	workerCount := min(s.cfg.MaxWorkers, len(channels)*len(recipients))

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if ctx.Err() != nil {
					errCh <- ctx.Err()
					continue
				}
				if err := s.processDelivery(ctx, event, definition, job); err != nil {
					errCh <- err
				}
			}
		}()
	}

	for _, channel := range channels {
		templateCode := templateCodeForChannel(definition, channel)
		for _, recipient := range recipients {
			jobs <- deliveryJob{
				event:        event,
				channel:      channel,
				templateCode: templateCode,
				recipient:    recipient,
				locale:       opts.Locale,
			}
		}
	}
	close(jobs)
	wg.Wait()
	close(errCh)

	failed := false
	for err := range errCh {
		if err != nil {
			failed = true
			s.logger.Error("dispatcher delivery failed", logger.Field{Key: "error", Value: err})
		}
	}

	status := domain.EventStatusProcessed
	if failed {
		status = domain.EventStatusFailed
	}
	if s.events != nil {
		_ = s.events.UpdateStatus(ctx, event.ID, status)
	}
	if failed {
		return errors.New("dispatcher: one or more deliveries failed")
	}
	return nil
}

func (s *Service) resolveSecrets(ctx context.Context, event *domain.NotificationEvent, job deliveryJob, messenger adapters.Messenger, overrideProvider string) (map[string][]byte, error) {
	channelType, provider := adapters.ParseChannel(job.channel)
	if overrideProvider != "" {
		provider = overrideProvider
	}
	if provider == "" {
		provider = messenger.Name()
	}
	if s.secrets == nil {
		if s.allowFallback(job.recipient, event) {
			return nil, nil
		}
		return nil, fmt.Errorf("dispatcher: secrets resolver not configured and fallback not allowed for recipient %s", job.recipient)
	}

	refs := []secrets.Reference{
		{Scope: secrets.ScopeUser, SubjectID: job.recipient, Channel: channelType, Provider: provider, Key: "default"},
	}
	if event != nil && strings.TrimSpace(event.TenantID) != "" {
		refs = append(refs, secrets.Reference{Scope: secrets.ScopeTenant, SubjectID: event.TenantID, Channel: channelType, Provider: provider, Key: "default"})
	}
	refs = append(refs, secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: channelType, Provider: provider, Key: "default"})

	resolved, err := s.secrets.Resolve(refs...)
	if err != nil && err != secrets.ErrNotFound {
		return nil, err
	}

	// Prefer user -> tenant -> system
	for _, ref := range refs {
		if val, ok := resolved[ref]; ok {
			return map[string][]byte{"default": val.Data}, nil
		}
	}

	if s.allowFallback(job.recipient, event) {
		return nil, nil
	}
	return nil, fmt.Errorf("dispatcher: no scoped secret for recipient %s and fallback not allowed", job.recipient)
}

func (s *Service) allowFallback(recipient string, event *domain.NotificationEvent) bool {
	if len(s.cfg.EnvFallbackAllowlist) == 0 {
		return false
	}
	for _, allowed := range s.cfg.EnvFallbackAllowlist {
		if allowed == recipient {
			return true
		}
		if event != nil && allowed == event.TenantID {
			return true
		}
	}
	return false
}

type deliveryJob struct {
	event        *domain.NotificationEvent
	channel      string
	templateCode string
	recipient    string
	locale       string
}

func (s *Service) processDelivery(ctx context.Context, event *domain.NotificationEvent, def *domain.NotificationDefinition, job deliveryJob) error {
	channelType, provider := adapters.ParseChannel(job.channel)
	inboxChannel := isInboxChannel(channelType)
	renderLocale := job.locale
	if renderLocale == "" && event != nil {
		if locale, ok := event.Context["locale"].(string); ok && locale != "" {
			renderLocale = locale
		}
	}

	preferredProvider := ""
	if allowed, reason, providerOverride, err := s.allowDelivery(ctx, event, def, job.recipient, channelType); err != nil {
		return fmt.Errorf("preferences evaluation: %w", err)
	} else if !allowed {
		s.logger.Debug("delivery skipped by preferences",
			logger.Field{Key: "recipient", Value: job.recipient},
			logger.Field{Key: "channel", Value: channelType},
			logger.Field{Key: "reason", Value: reason},
		)
		return nil
	} else if providerOverride != "" {
		preferredProvider = providerOverride
	}

	payload := cloneJSONMap(event.Context)
	if payload == nil {
		payload = make(domain.JSONMap)
	}
	attachments := adapters.AttachmentsFromValue(payload["attachments"])
	channelAttachments := adapters.ChannelAttachmentsFromValue(payload["channel_attachments"])
	if override := adapters.ChannelAttachmentsFor(channelAttachments, channelType); len(override) > 0 {
		attachments = override
	}
	payload["recipient"] = job.recipient
	payload["channel"] = channelType
	payload["provider"] = provider
	payload["definition"] = def.Metadata
	resolveTemplateOverrides(payload, channelType)

	renderResult, err := s.templates.Render(ctx, templates.RenderRequest{
		Code:    job.templateCode,
		Channel: channelType,
		Locale:  renderLocale,
		Data:    payload,
	})
	if err != nil {
		s.logger.Error("dispatcher render failed",
			logger.Field{Key: "template", Value: job.templateCode},
			logger.Field{Key: "channel", Value: channelType},
			logger.Field{Key: "recipient", Value: job.recipient},
			logger.Field{Key: "definition", Value: def.Code},
			logger.Field{Key: "event_id", Value: event.ID},
			logger.Field{Key: "error", Value: err},
		)
		s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, nil, "failed", provider, renderLocale, err))
		return fmt.Errorf("render template %s: %w", job.templateCode, err)
	}

	message := &domain.NotificationMessage{
		EventID:  event.ID,
		Channel:  channelType,
		Locale:   renderResult.Locale,
		Subject:  renderResult.Subject,
		Body:     renderResult.Body,
		Receiver: job.recipient,
		Status:   domain.MessageStatusPending,
		Metadata: renderResult.Metadata,
	}
	applyChannelOverrides(payload, channelType, message)
	if s.messages != nil {
		if err := s.messages.Create(ctx, message); err != nil {
			s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, message, "failed", provider, renderLocale, err))
			return fmt.Errorf("persist message: %w", err)
		}
	}

	if inboxChannel {
		if s.inbox == nil {
			s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, message, "failed", provider, renderLocale, errors.New("inbox service not configured")))
			return errors.New("dispatcher: inbox channel requested but inbox service is not configured")
		}
		if err := s.handleInboxDelivery(ctx, message); err != nil {
			s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, message, "failed", provider, renderLocale, err))
			return err
		}
		s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, message, "delivered", provider, renderLocale, nil))
		return nil
	}
	// TODO: We should support multi-channel deliveries
	routeChannel := job.channel
	if preferredProvider != "" {
		routeChannel = fmt.Sprintf("%s:%s", channelType, preferredProvider)
	}
	candidates := s.registry.List(routeChannel)
	if len(candidates) == 0 {
		return fmt.Errorf("route channel %s: %w", routeChannel, adapters.ErrAdapterNotFound)
	}

	var success bool
	var lastErr error
	var lastProvider string

	for _, messenger := range candidates {
		resolvedAttachments := attachments
		if s.attachments != nil && len(attachments) > 0 {
			resolved, err := s.attachments.Resolve(ctx, adapters.AttachmentJob{
				Channel:        channelType,
				Provider:       messenger.Name(),
				Recipient:      job.recipient,
				EventID:        event.ID.String(),
				DefinitionCode: def.Code,
			}, attachments)
			if err != nil {
				lastErr = err
				lastProvider = messenger.Name()
				continue
			}
			resolvedAttachments = resolved
		}

		secretPayload, err := s.resolveSecrets(ctx, event, job, messenger, preferredProvider)
		if err != nil {
			lastErr = err
			lastProvider = messenger.Name()
			continue
		}

		sendMsg := adapters.Message{
			ID:          message.ID.String(),
			Channel:     channelType,
			Provider:    messenger.Name(),
			Subject:     message.Subject,
			Body:        message.Body,
			To:          message.Receiver,
			Attachments: resolvedAttachments,
			Metadata: map[string]any{
				"event_id":        event.ID.String(),
				"definition_code": def.Code,
			},
			Locale: renderResult.Locale,
		}
		if len(secretPayload) > 0 {
			sendMsg.Metadata["secrets"] = secretPayload
		}
		if len(message.Metadata) > 0 {
			if sendMsg.Metadata == nil {
				sendMsg.Metadata = make(map[string]any)
			}
			for k, v := range message.Metadata {
				if _, exists := sendMsg.Metadata[k]; exists {
					continue
				}
				sendMsg.Metadata[k] = v
			}
		}

		// Use a copy so per-adapter status updates don't clobber each other mid-loop.
		msgCopy := *message
		if err := s.deliverWithRetries(ctx, messenger, &msgCopy, sendMsg); err != nil {
			lastErr = err
			lastProvider = messenger.Name()
			continue
		}
		success = true
		lastProvider = messenger.Name()
	}

	if s.messages != nil {
		if success {
			message.Status = domain.MessageStatusDelivered
		} else {
			message.Status = domain.MessageStatusFailed
		}
		_ = s.messages.Update(ctx, message)
	}

	if !success {
		s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, message, "failed", lastProvider, renderResult.Locale, lastErr))
		return lastErr
	}
	s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, message, "delivered", lastProvider, renderResult.Locale, nil))
	return nil
}

func (s *Service) deliverWithRetries(ctx context.Context, messenger adapters.Messenger, message *domain.NotificationMessage, sendMsg adapters.Message) error {
	var lastErr error
	for attempt := 1; attempt <= s.cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		lastErr = messenger.Send(ctx, sendMsg)
		if lastErr == nil {
			_ = s.recordAttempt(ctx, messenger.Name(), message, domain.AttemptStatusSucceeded, "", attempt)
			message.Status = domain.MessageStatusDelivered
			if s.messages != nil {
				_ = s.messages.Update(ctx, message)
			}
			return nil
		}
		s.logger.Warn("delivery error", logger.Field{Key: "attempt", Value: attempt}, logger.Field{Key: "error", Value: lastErr})
		_ = s.recordAttempt(ctx, messenger.Name(), message, domain.AttemptStatusFailed, lastErr.Error(), attempt)
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
	message.Status = domain.MessageStatusFailed
	if s.messages != nil {
		_ = s.messages.Update(ctx, message)
	}
	return fmt.Errorf("dispatcher: delivery failed after %d attempts: %w", s.cfg.MaxRetries, lastErr)
}

func (s *Service) recordAttempt(ctx context.Context, adapterName string, message *domain.NotificationMessage, status, errMsg string, attempt int) error {
	if s.attempts == nil {
		return nil
	}
	record := &domain.DeliveryAttempt{
		MessageID: message.ID,
		Adapter:   adapterName,
		Status:    status,
		Error:     errMsg,
		Payload: domain.JSONMap{
			"attempt": attempt,
		},
	}
	return s.attempts.Create(ctx, record)
}

func (s *Service) buildDeliveryActivity(event *domain.NotificationEvent, def *domain.NotificationDefinition, job deliveryJob, message *domain.NotificationMessage, status, provider, locale string, err error) activity.Event {
	defCode := ""
	actorID := ""
	tenantID := ""
	contextCopy := domain.JSONMap{}
	objectID := ""

	if def != nil {
		defCode = def.Code
	}
	if event != nil {
		defCode = event.DefinitionCode
		actorID = event.ActorID
		tenantID = event.TenantID
		objectID = event.ID.String()
		if len(event.Context) > 0 {
			contextCopy = cloneJSONMap(event.Context)
			sanitizeContext(contextCopy)
		}
	}
	if message != nil {
		objectID = message.ID.String()
	}

	meta := map[string]any{
		"channel":   job.channel,
		"provider":  provider,
		"locale":    locale,
		"status":    status,
		"context":   contextCopy,
		"template":  job.templateCode,
		"recipient": job.recipient,
	}
	if err != nil {
		meta["error"] = err.Error()
	}
	if message != nil {
		meta["message_status"] = message.Status
	}
	recipients := []string(nil)
	if job.recipient != "" {
		recipients = []string{job.recipient}
	}

	return activity.Event{
		Verb:           fmt.Sprintf("notification.%s", status),
		ActorID:        actorID,
		UserID:         job.recipient,
		TenantID:       tenantID,
		ObjectType:     "notification_message",
		ObjectID:       objectID,
		Channel:        job.channel,
		DefinitionCode: defCode,
		Recipients:     recipients,
		Metadata:       meta,
	}
}

func templateCodeForChannel(def *domain.NotificationDefinition, ch string) string {
	if def == nil {
		return ""
	}
	chType, _ := adapters.ParseChannel(ch)
	for _, entry := range def.TemplateKeys {
		parts := strings.Split(entry, ":")
		if len(parts) == 2 {
			if strings.EqualFold(parts[0], chType) {
				return parts[1]
			}
		}
	}
	if len(def.TemplateKeys) > 0 {
		return def.TemplateKeys[0]
	}
	return def.Code
}

func cloneJSONMap(src domain.JSONMap) domain.JSONMap {
	if len(src) == 0 {
		return nil
	}
	dst := make(domain.JSONMap, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func sanitizeContext(ctx domain.JSONMap) {
	if len(ctx) == 0 {
		return
	}
	delete(ctx, "attachments")
	delete(ctx, "channel_attachments")
}

func applyChannelOverrides(payload domain.JSONMap, channel string, message *domain.NotificationMessage) {
	if message.Metadata == nil {
		message.Metadata = make(domain.JSONMap)
	}
	if link := firstString(payload, "url", "action_url"); link != "" {
		message.Metadata["action_url"] = link
	}
	if manifest := firstString(payload, "manifest_url"); manifest != "" {
		message.Metadata["manifest_url"] = manifest
	}

	overrides := extractOverrides(payload, channel)
	if len(overrides) == 0 {
		return
	}
	if subject, ok := overrides["subject"].(string); ok && strings.TrimSpace(subject) != "" {
		message.Subject = subject
	}
	if body, ok := overrides["body"].(string); ok && strings.TrimSpace(body) != "" {
		message.Body = body
	}
	if htmlBody, ok := overrides["html_body"].(string); ok && strings.TrimSpace(htmlBody) != "" {
		message.Metadata["html_body"] = htmlBody
	}
	if textBody, ok := overrides["text_body"].(string); ok && strings.TrimSpace(textBody) != "" {
		message.Metadata["text_body"] = textBody
	}
	if action, ok := overrides["action_url"].(string); ok && strings.TrimSpace(action) != "" {
		message.Metadata["action_url"] = action
	}
	if icon, ok := overrides["icon"].(string); ok && strings.TrimSpace(icon) != "" {
		message.Metadata["icon"] = icon
	}
	if badge, ok := overrides["badge"].(string); ok && strings.TrimSpace(badge) != "" {
		message.Metadata["badge"] = badge
	}
	if cta, ok := overrides["cta_label"].(string); ok && strings.TrimSpace(cta) != "" {
		message.Metadata["cta_label"] = cta
	}
}

func resolveTemplateOverrides(payload domain.JSONMap, channel string) {
	overrides := extractOverrides(payload, channel)
	if len(overrides) == 0 {
		return
	}
	// ensure map exists for renderer helpers
	if payload == nil {
		payload = make(domain.JSONMap)
	}
	if label := firstString(overrides, "cta_label"); label != "" {
		payload["cta_label"] = label
	}
	if link := firstString(overrides, "action_url"); link != "" {
		payload["action_url"] = link
	}
}

func firstString(m map[string]any, keys ...string) string {
	if len(m) == 0 {
		return ""
	}
	for _, key := range keys {
		if val, ok := m[key]; ok {
			switch v := val.(type) {
			case string:
				if s := strings.TrimSpace(v); s != "" {
					return s
				}
			default:
				if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func extractOverrides(payload domain.JSONMap, channel string) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	raw, ok := payload["channel_overrides"]
	if !ok {
		return nil
	}
	switch ov := raw.(type) {
	case map[string]any:
		if ch, ok := ov[channel]; ok {
			if m, ok := ch.(map[string]any); ok {
				return m
			}
		}
	case map[string]map[string]any:
		if m, ok := ov[channel]; ok {
			return m
		}
	}
	return nil
}

func (s *Service) handleInboxDelivery(ctx context.Context, message *domain.NotificationMessage) error {
	if message == nil {
		return errors.New("dispatcher: message is required for inbox delivery")
	}
	if message.Metadata != nil {
		if link, ok := message.Metadata["action_url"]; !ok || link == "" {
			message.Metadata["action_url"] = ""
		}
	}
	if err := s.inbox.DeliverFromMessage(ctx, message); err != nil {
		return fmt.Errorf("dispatcher: inbox delivery failed: %w", err)
	}
	message.Status = domain.MessageStatusDelivered
	if s.messages != nil {
		_ = s.messages.Update(ctx, message)
	}
	return nil
}

func (s *Service) allowDelivery(ctx context.Context, event *domain.NotificationEvent, def *domain.NotificationDefinition, recipient, channel string) (bool, string, string, error) {
	if s.preferences == nil || def == nil || event == nil {
		return true, "", "", nil
	}
	scopes := buildPreferenceScopes(event, recipient, def.Code, channel)
	req := prefsvc.EvaluationRequest{
		DefinitionCode: def.Code,
		Channel:        channel,
		Scopes:         scopes,
		Subscriptions:  eventSubscriptions(event),
	}
	if !event.ScheduledAt.IsZero() {
		req.Timestamp = event.ScheduledAt
	}
	result, err := s.preferences.Evaluate(ctx, req)
	if err != nil {
		return false, "", "", err
	}
	if !result.Allowed {
		return false, result.Reason, result.Provider, nil
	}
	return true, "", result.Provider, nil
}

func buildPreferenceScopes(event *domain.NotificationEvent, recipient, definitionCode, channel string) []pkgoptions.PreferenceScopeRef {
	var scopes []pkgoptions.PreferenceScopeRef
	if recipient != "" {
		scopes = append(scopes, pkgoptions.PreferenceScopeRef{
			Scope:          opts.NewScope("user", opts.ScopePriorityUser),
			SubjectType:    "user",
			SubjectID:      recipient,
			DefinitionCode: definitionCode,
			Channel:        channel,
		})
	}
	if event != nil && event.TenantID != "" {
		scopes = append(scopes, pkgoptions.PreferenceScopeRef{
			Scope:          opts.NewScope("tenant", opts.ScopePriorityTenant),
			SubjectType:    "tenant",
			SubjectID:      event.TenantID,
			DefinitionCode: definitionCode,
			Channel:        channel,
		})
	}
	scopes = append(scopes, pkgoptions.PreferenceScopeRef{
		Scope:          opts.NewScope("system", opts.ScopePrioritySystem),
		SubjectType:    "system",
		SubjectID:      "default",
		DefinitionCode: definitionCode,
		Channel:        channel,
	})
	return scopes
}

func eventSubscriptions(event *domain.NotificationEvent) []string {
	if event == nil || len(event.Context) == 0 {
		return nil
	}
	raw, ok := event.Context["subscriptions"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case domain.StringList:
		return append([]string(nil), []string(v)...)
	case []any:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			if str, ok := entry.(string); ok && strings.TrimSpace(str) != "" {
				out = append(out, strings.TrimSpace(str))
			}
		}
		return out
	default:
		return nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isInboxChannel(channel string) bool {
	switch channel {
	case "inbox", "in-app", "inapp", "in_app":
		return true
	default:
		return false
	}
}
