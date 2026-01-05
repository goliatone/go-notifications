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
	"github.com/goliatone/go-notifications/pkg/links"
	pkgoptions "github.com/goliatone/go-notifications/pkg/options"
	prefsvc "github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/retry"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/goliatone/go-notifications/pkg/templates"
	opts "github.com/goliatone/go-options"
	"github.com/google/uuid"
)

// Dependencies groups the repositories/services required by the dispatcher.
type inboxDeliverer interface {
	DeliverFromMessage(ctx context.Context, msg *domain.NotificationMessage) error
}

type Dependencies struct {
	Definitions  store.NotificationDefinitionRepository
	Events       store.NotificationEventRepository
	Messages     store.NotificationMessageRepository
	Attempts     store.DeliveryAttemptRepository
	Templates    *templates.Service
	Registry     *adapters.Registry
	Attachments  adapters.AttachmentResolver
	LinkBuilder  links.LinkBuilder
	LinkStore    links.LinkStore
	LinkObserver links.LinkObserver
	LinkPolicy   links.FailurePolicy
	Logger       logger.Logger
	Config       config.DispatcherConfig
	Preferences  *prefsvc.Service
	Inbox        inboxDeliverer
	Secrets      secrets.Resolver
	Backoff      retry.Backoff
	Activity     activity.Hooks
}

// Service expands events into rendered messages and routes them to adapters.
type Service struct {
	definitions  store.NotificationDefinitionRepository
	events       store.NotificationEventRepository
	messages     store.NotificationMessageRepository
	attempts     store.DeliveryAttemptRepository
	templates    *templates.Service
	registry     *adapters.Registry
	attachments  adapters.AttachmentResolver
	linkBuilder  links.LinkBuilder
	linkStore    links.LinkStore
	linkObserver links.LinkObserver
	linkPolicy   links.FailurePolicy
	logger       logger.Logger
	cfg          config.DispatcherConfig
	preferences  *prefsvc.Service
	inbox        inboxDeliverer
	secrets      secrets.Resolver
	backoff      retry.Backoff
	activity     activity.Hooks
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
		deps.Logger = logger.Default()
	}
	if deps.Backoff == nil {
		deps.Backoff = retry.DefaultBackoff()
	}
	if deps.LinkStore == nil {
		deps.LinkStore = &links.NopStore{}
	}
	if deps.LinkObserver == nil {
		deps.LinkObserver = &links.NopObserver{}
	}

	if deps.Config.MaxWorkers <= 0 {
		deps.Config.MaxWorkers = 4
	}

	if deps.Config.MaxRetries <= 0 {
		deps.Config.MaxRetries = 3
	}

	linkPolicy := normalizeLinkPolicy(deps.LinkPolicy)

	return &Service{
		definitions:  deps.Definitions,
		events:       deps.Events,
		messages:     deps.Messages,
		attempts:     deps.Attempts,
		templates:    deps.Templates,
		registry:     deps.Registry,
		attachments:  deps.Attachments,
		linkBuilder:  deps.LinkBuilder,
		linkStore:    deps.LinkStore,
		linkObserver: deps.LinkObserver,
		linkPolicy:   linkPolicy,
		logger:       deps.Logger,
		cfg:          deps.Config,
		preferences:  deps.Preferences,
		inbox:        deps.Inbox,
		secrets:      deps.Secrets,
		backoff:      deps.Backoff,
		activity:     deps.Activity,
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
			s.logger.Error("dispatcher delivery failed", "error", err)
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
			"recipient", job.recipient,
			"channel", channelType,
			"reason", reason,
		)
		return nil
	} else if providerOverride != "" {
		preferredProvider = providerOverride
	}

	messageID := uuid.New()
	payload := cloneJSONMap(event.Context)
	if payload == nil {
		payload = make(domain.JSONMap)
	}
	basePayload := cloneJSONMap(payload)
	attachments := adapters.AttachmentsFromValue(payload["attachments"])
	channelAttachments := adapters.ChannelAttachmentsFromValue(payload["channel_attachments"])
	if override := adapters.ChannelAttachmentsFor(channelAttachments, channelType); len(override) > 0 {
		attachments = override
	}
	payload["recipient"] = job.recipient
	payload["channel"] = channelType
	payload["provider"] = provider
	payload["definition"] = def.Metadata
	applyChannelOverridesToPayload(payload, channelType)
	normalizeLinkPayload(payload)

	resolvedProvider := provider
	if preferredProvider != "" {
		resolvedProvider = preferredProvider
	}
	linkReq, resolvedLinks, builderOK, err := s.resolveLinks(ctx, event, def, job, basePayload, payload, channelType, resolvedProvider, renderLocale, messageID)
	if err != nil {
		s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, nil, "failed", resolvedProvider, renderLocale, err))
		return err
	}
	applyResolvedLinksToPayload(payload, resolvedLinks)

	renderResult, err := s.templates.Render(ctx, templates.RenderRequest{
		Code:    job.templateCode,
		Channel: channelType,
		Locale:  renderLocale,
		Data:    payload,
	})
	if err != nil {
		s.logger.Error("dispatcher render failed",
			"template", job.templateCode,
			"channel", channelType,
			"recipient", job.recipient,
			"definition", def.Code,
			"event_id", event.ID,
			"error", err,
		)
		s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, nil, "failed", provider, renderLocale, err))
		return fmt.Errorf("render template %s: %w", job.templateCode, err)
	}

	message := &domain.NotificationMessage{
		RecordMeta: domain.RecordMeta{ID: messageID},
		EventID:    event.ID,
		Channel:    channelType,
		Locale:     renderResult.Locale,
		Subject:    renderResult.Subject,
		Body:       renderResult.Body,
		Receiver:   job.recipient,
		Status:     domain.MessageStatusPending,
		Metadata:   renderResult.Metadata,
	}
	applyChannelOverrides(payload, channelType, message)
	applyResolvedLinksToMessage(message, resolvedLinks)
	if builderOK {
		if err := s.invokeLinkHooks(ctx, linkReq, resolvedLinks); err != nil {
			s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, message, "failed", resolvedProvider, renderLocale, err))
			return err
		}
	}
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
		s.logger.Warn("delivery error", "attempt", attempt, "error", lastErr)
		_ = s.recordAttempt(ctx, messenger.Name(), message, domain.AttemptStatusFailed, lastErr.Error(), attempt)
		var delay time.Duration
		if s.backoff != nil {
			delay = s.backoff.Next(attempt)
		} else {
			delay = retry.DefaultBackoff().Next(attempt)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
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

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
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

func applyChannelOverridesToPayload(payload domain.JSONMap, channel string) {
	overrides := extractOverrides(payload, channel)
	if len(overrides) == 0 {
		return
	}
	// ensure map exists for renderer helpers
	if payload == nil {
		payload = make(domain.JSONMap)
	}
	if value := firstString(overrides, "cta_label"); value != "" {
		payload["cta_label"] = value
	}
	if value := firstString(overrides, "icon"); value != "" {
		payload["icon"] = value
	}
	if value := firstString(overrides, "badge"); value != "" {
		payload["badge"] = value
	}
	if value := firstString(overrides, links.ResolvedURLManifestKey); value != "" {
		payload[links.ResolvedURLManifestKey] = value
	}
	if action := firstString(overrides, links.ResolvedURLActionKey); action != "" {
		payload[links.ResolvedURLActionKey] = action
	} else if url := firstString(overrides, links.ResolvedURLKey); url != "" {
		payload[links.ResolvedURLActionKey] = url
	}
	if url := firstString(overrides, links.ResolvedURLKey); url != "" {
		payload[links.ResolvedURLKey] = url
	}
}

func normalizeLinkPayload(payload domain.JSONMap) {
	if len(payload) == 0 {
		return
	}
	if firstString(payload, links.ResolvedURLActionKey) == "" {
		if url := firstString(payload, links.ResolvedURLKey); url != "" {
			payload[links.ResolvedURLActionKey] = url
		}
	}
}

func normalizeLinkPolicy(policy links.FailurePolicy) links.FailurePolicy {
	policy.Builder = normalizeFailureMode(policy.Builder, links.FailureStrict)
	policy.Store = normalizeFailureMode(policy.Store, links.FailureLenient)
	policy.Observer = normalizeFailureMode(policy.Observer, links.FailureLenient)
	return policy
}

func normalizeFailureMode(mode, fallback links.FailureMode) links.FailureMode {
	if mode == "" {
		return fallback
	}
	return mode
}

func (s *Service) resolveLinks(ctx context.Context, event *domain.NotificationEvent, def *domain.NotificationDefinition, job deliveryJob, basePayload, payload domain.JSONMap, channel, provider, locale string, messageID uuid.UUID) (links.LinkRequest, links.ResolvedLinks, bool, error) {
	// Precedence: overrides > original (builder wins later).
	baseResolved := mergeResolvedLinks(
		resolvedLinksFromPayload(basePayload),
		resolvedLinksFromOverrides(basePayload, channel),
	)
	baseResolved = normalizeResolvedLinks(baseResolved)
	if s.linkBuilder == nil {
		return links.LinkRequest{}, baseResolved, false, nil
	}
	req := links.LinkRequest{
		EventID:      event.ID.String(),
		Definition:   def.Code,
		Recipient:    job.recipient,
		Channel:      channel,
		Provider:     provider,
		TemplateCode: job.templateCode,
		MessageID:    messageID.String(),
		Locale:       locale,
		Payload:      cloneJSONMap(payload),
		Metadata:     nil,
		ResolvedURLs: resolvedURLsFromPayload(payload),
	}
	resolved, err := s.linkBuilder.Build(ctx, req)
	if err != nil {
		if s.linkPolicy.Builder == links.FailureLenient {
			s.logger.Warn("link builder failed; continuing with pass-through links",
				"definition", def.Code,
				"channel", job.channel,
				"recipient", job.recipient,
				"error", err,
			)
			return req, baseResolved, false, nil
		}
		return req, links.ResolvedLinks{}, false, err
	}
	resolved = normalizeResolvedLinks(resolved)
	merged := mergeResolvedLinks(baseResolved, resolved)
	merged = normalizeResolvedLinks(merged)
	merged = ensureResolvedLinkRecords(req, merged)
	return req, merged, true, nil
}

func (s *Service) invokeLinkHooks(ctx context.Context, req links.LinkRequest, resolved links.ResolvedLinks) error {
	if s.linkStore != nil {
		if err := s.linkStore.Save(ctx, resolved.Records); err != nil {
			if s.linkPolicy.Store == links.FailureLenient {
				s.logger.Warn("link store save failed; continuing",
					"definition", req.Definition,
					"channel", req.Channel,
					"recipient", req.Recipient,
					"error", err,
				)
			} else {
				return err
			}
		}
	}
	if s.linkObserver != nil {
		s.linkObserver.OnLinksResolved(ctx, links.LinkResolution{
			Request:  req,
			Resolved: resolved,
		})
	}
	return nil
}

func resolvedLinksFromPayload(payload domain.JSONMap) links.ResolvedLinks {
	return resolvedLinksFromMap(payload)
}

func resolvedLinksFromOverrides(payload domain.JSONMap, channel string) links.ResolvedLinks {
	overrides := extractOverrides(payload, channel)
	return resolvedLinksFromMap(overrides)
}

func resolvedLinksFromMap(payload map[string]any) links.ResolvedLinks {
	if len(payload) == 0 {
		return links.ResolvedLinks{}
	}
	return links.ResolvedLinks{
		ActionURL:   firstString(payload, links.ResolvedURLActionKey, links.ResolvedURLKey),
		ManifestURL: firstString(payload, links.ResolvedURLManifestKey),
		URL:         firstString(payload, links.ResolvedURLKey),
	}
}

func normalizeResolvedLinks(resolved links.ResolvedLinks) links.ResolvedLinks {
	if resolved.ActionURL == "" && resolved.URL != "" {
		resolved.ActionURL = resolved.URL
	}
	return resolved
}

func ensureResolvedLinkRecords(req links.LinkRequest, resolved links.ResolvedLinks) links.ResolvedLinks {
	if len(resolved.Records) > 0 {
		return resolved
	}
	records := buildLinkRecords(req, resolved)
	if len(records) > 0 {
		resolved.Records = records
	}
	return resolved
}

func buildLinkRecords(req links.LinkRequest, resolved links.ResolvedLinks) []links.LinkRecord {
	type candidate struct {
		key string
		url string
	}
	candidates := []candidate{
		{key: links.ResolvedURLActionKey, url: resolved.ActionURL},
		{key: links.ResolvedURLManifestKey, url: resolved.ManifestURL},
		{key: links.ResolvedURLKey, url: resolved.URL},
	}
	seen := make(map[string]struct{}, len(candidates))
	records := make([]links.LinkRecord, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.url == "" {
			continue
		}
		if _, ok := seen[candidate.url]; ok {
			continue
		}
		seen[candidate.url] = struct{}{}
		record := links.LinkRecord{
			URL:        candidate.url,
			Channel:    req.Channel,
			Recipient:  req.Recipient,
			MessageID:  req.MessageID,
			Definition: req.Definition,
		}
		if len(resolved.Metadata) > 0 {
			record.Metadata = cloneAnyMap(resolved.Metadata)
		}
		if record.Metadata == nil {
			record.Metadata = map[string]any{"link_key": candidate.key}
		} else if _, exists := record.Metadata["link_key"]; !exists {
			record.Metadata["link_key"] = candidate.key
		}
		records = append(records, record)
	}
	return records
}

func mergeResolvedLinks(base, override links.ResolvedLinks) links.ResolvedLinks {
	if override.ActionURL != "" {
		base.ActionURL = override.ActionURL
	}
	if override.ManifestURL != "" {
		base.ManifestURL = override.ManifestURL
	}
	if override.URL != "" {
		base.URL = override.URL
	}
	if len(override.Metadata) > 0 {
		if base.Metadata == nil {
			base.Metadata = make(map[string]any, len(override.Metadata))
		}
		for key, value := range override.Metadata {
			base.Metadata[key] = value
		}
	}
	if len(override.Records) > 0 {
		base.Records = override.Records
	}
	return base
}

func applyResolvedLinksToPayload(payload domain.JSONMap, resolved links.ResolvedLinks) {
	if payload == nil {
		return
	}
	if resolved.ActionURL != "" {
		payload[links.ResolvedURLActionKey] = resolved.ActionURL
	}
	if resolved.ManifestURL != "" {
		payload[links.ResolvedURLManifestKey] = resolved.ManifestURL
	}
	if resolved.URL != "" {
		payload[links.ResolvedURLKey] = resolved.URL
	}
}

func applyResolvedLinksToMessage(message *domain.NotificationMessage, resolved links.ResolvedLinks) {
	if message == nil {
		return
	}
	if resolved.ActionURL != "" {
		message.ActionURL = resolved.ActionURL
	}
	if resolved.ManifestURL != "" {
		message.ManifestURL = resolved.ManifestURL
	}
	if resolved.URL != "" {
		message.URL = resolved.URL
	}
	if len(resolved.Metadata) > 0 {
		if message.Metadata == nil {
			message.Metadata = make(domain.JSONMap, len(resolved.Metadata))
		}
		for key, value := range resolved.Metadata {
			message.Metadata[key] = value
		}
	}
}

func resolvedURLsFromPayload(payload domain.JSONMap) map[string]string {
	if len(payload) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range payload {
		if !links.IsResolvedURLKey(key) {
			continue
		}
		if str, ok := stringValue(value); ok {
			out[key] = str
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringValue(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			return s, true
		}
	default:
		if value == nil {
			return "", false
		}
		if s := strings.TrimSpace(fmt.Sprint(value)); s != "" {
			return s, true
		}
	}
	return "", false
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
