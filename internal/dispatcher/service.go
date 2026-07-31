package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
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
	"github.com/goliatone/go-notifications/pkg/persistencepolicy"
	prefsvc "github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/receipts"
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
	Persistence  persistencepolicy.Policy
	Privacy      privacy.Policy
	Diagnostic   privacy.DiagnosticSink
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
	persistence  persistencepolicy.Policy
	privacy      privacy.Policy
	diagnostic   privacy.DiagnosticSink
}

// DispatchOptions allow callers to override channels/locales.
type DispatchOptions struct {
	Channels         []string
	Locale           string
	Transient        map[string]any
	RetryFailedOnly  bool
	RetryOperationID uuid.UUID
}

var (
	ErrMissingDefinitions = errors.New("dispatcher: definition repository is required")
	ErrMissingTemplates   = errors.New("dispatcher: templates service is required")
	ErrMissingRegistry    = errors.New("dispatcher: adapter registry is required")
	ErrInvalidConfig      = errors.New("dispatcher: invalid config")
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
	if deps.Persistence == nil {
		deps.Persistence = persistencepolicy.DefinitionPolicy{}
	}
	if deps.Privacy == nil {
		deps.Privacy = privacy.DefaultPolicy{}
	}
	if deps.Diagnostic == nil {
		deps.Diagnostic = privacy.NopDiagnosticSink{}
	}

	if deps.Config.MaxWorkers <= 0 {
		return nil, fmt.Errorf("%w: max_workers must be > 0", ErrInvalidConfig)
	}
	if deps.Config.MaxAttempts <= 0 {
		return nil, fmt.Errorf("%w: max_attempts must be > 0", ErrInvalidConfig)
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
		persistence:  deps.Persistence,
		privacy:      deps.Privacy,
		diagnostic:   deps.Diagnostic,
	}, nil
}

// Dispatch expands the stored event into deliveries using the configured adapters.
func (s *Service) Dispatch(ctx context.Context, event *domain.NotificationEvent, opts DispatchOptions) error {
	_, err := s.DispatchWithReceipt(ctx, event, opts)
	return err
}

// DispatchWithReceipt expands an event and returns deterministic, privacy-safe
// recipient/channel/provider outcomes.
func (s *Service) DispatchWithReceipt(ctx context.Context, event *domain.NotificationEvent, opts DispatchOptions) (receipts.DispatchReceipt, error) {
	receipt := receipts.DispatchReceipt{}
	if event != nil {
		receipt.EventID = event.ID
		receipt.CorrelationID = event.CorrelationID
		receipt.RequestID = event.RequestID
	}
	rawErr := s.dispatch(ctx, event, opts)
	receipt.Outcomes = s.reconstructOutcomes(ctx, event, opts, rawErr)
	receipt.Status = aggregateReceiptStatus(receipt.Outcomes, rawErr)
	if event != nil && s.events != nil {
		if status := receiptEventStatus(receipt.Status); status != "" {
			if statusErr := s.events.UpdateStatus(ctx, event.ID, status); statusErr != nil {
				rawErr = errors.Join(rawErr, fmt.Errorf("dispatcher: persist receipt status: %w", statusErr))
			}
		}
	}
	if rawErr == nil {
		return receipt, nil
	}
	s.diagnostic.Report(ctx, privacy.DiagnosticEvent{
		Operation: "dispatcher.dispatch",
		EventID:   receipt.EventID.String(),
		Cause:     rawErr,
	})
	safe := s.privacy.SafeError(rawErr)
	return receipt, safe
}

// ReceiptForEvent reconstructs the current privacy-safe receipt from durable
// message and attempt state without rendering or delivering again.
func (s *Service) ReceiptForEvent(ctx context.Context, event *domain.NotificationEvent) (receipts.DispatchReceipt, error) {
	if event == nil {
		return receipts.DispatchReceipt{}, privacy.SafeError{
			Category: "validation",
			Code:     "event_required",
			Message:  "notification event is required",
		}
	}
	receipt := receipts.DispatchReceipt{
		EventID:       event.ID,
		PublicationID: event.PublicationID,
		CorrelationID: event.CorrelationID,
		RequestID:     event.RequestID,
		Outcomes:      s.reconstructOutcomes(ctx, event, DispatchOptions{}, nil),
	}
	receipt.Status = receiptStatusForEvent(event.Status, receipt.Outcomes)
	return receipt, nil
}

func (s *Service) dispatch(ctx context.Context, event *domain.NotificationEvent, opts DispatchOptions) error {
	plan, err := s.buildDispatchPlan(ctx, event, opts)
	if err != nil {
		return err
	}
	deliveryErr := s.executeJobs(ctx, event, plan.definition, plan.jobs)
	return s.completeDispatch(ctx, event, deliveryErr)
}

type dispatchPlan struct {
	definition *domain.NotificationDefinition
	jobs       []deliveryJob
}

func (s *Service) buildDispatchPlan(ctx context.Context, event *domain.NotificationEvent, opts DispatchOptions) (dispatchPlan, error) {
	if event == nil {
		return dispatchPlan{}, errors.New("dispatcher: event is required")
	}
	definition, err := s.definitions.GetByCode(ctx, event.DefinitionCode)
	if err != nil {
		return dispatchPlan{}, fmt.Errorf("dispatcher: load definition: %w", err)
	}
	decision, err := s.persistence.Resolve(ctx, definition)
	if err != nil {
		return dispatchPlan{}, fmt.Errorf("dispatcher: resolve persistence policy: %w", err)
	}
	decision = persistencepolicy.WithTransientOverlay(decision, len(opts.Transient) > 0 || event.TransientDependent)
	channels := opts.Channels
	if len(channels) == 0 {
		channels = event.Channels
	}
	if len(channels) == 0 {
		channels = definition.Channels
	}
	if opts.Locale == "" {
		opts.Locale = event.Locale
	}
	if len(channels) == 0 {
		return dispatchPlan{}, errors.New("dispatcher: no channels configured")
	}
	if len(event.Recipients) == 0 {
		return dispatchPlan{}, errors.New("dispatcher: event has no recipients")
	}
	existingMessages := make(map[string]*domain.NotificationMessage)
	if s.messages != nil {
		existing, listErr := s.messages.ListByEvent(ctx, event.ID)
		if listErr != nil {
			return dispatchPlan{}, fmt.Errorf("dispatcher: load existing messages: %w", listErr)
		}
		for idx := range existing {
			copy := existing[idx]
			existingMessages[copy.Receiver+"\x00"+copy.Channel] = &copy
		}
	}
	retryProviders, err := s.retryProviderPlans(ctx, opts, existingMessages)
	if err != nil {
		return dispatchPlan{}, err
	}
	jobs := buildDeliveryJobs(event, definition, channels, decision, existingMessages, retryProviders, opts)
	return dispatchPlan{definition: definition, jobs: jobs}, nil
}

func (s *Service) retryProviderPlans(
	ctx context.Context,
	opts DispatchOptions,
	existing map[string]*domain.NotificationMessage,
) (map[string][]string, error) {
	if !opts.RetryFailedOnly {
		return nil, nil
	}
	if s.messages == nil || s.attempts == nil {
		return nil, errors.New("dispatcher: retry requires message and attempt repositories")
	}
	plans := make(map[string][]string)
	sameOperationLineage := false
	for key, message := range existing {
		providers, sameOperation, err := s.retryProviderPlan(ctx, message, opts.RetryOperationID)
		if err != nil {
			return nil, err
		}
		if sameOperation {
			sameOperationLineage = true
		}
		if len(providers) > 0 {
			plans[key] = providers
		}
	}
	if len(plans) == 0 && !sameOperationLineage {
		return nil, errors.New("dispatcher: no failed outcomes eligible for retry")
	}
	return plans, nil
}

func (s *Service) retryProviderPlan(
	ctx context.Context,
	message *domain.NotificationMessage,
	operationID uuid.UUID,
) ([]string, bool, error) {
	attempts, err := s.attempts.ListByMessage(ctx, message.ID)
	if err != nil {
		return nil, false, fmt.Errorf("dispatcher: load delivery attempts: %w", err)
	}
	providers := retryPlanProviders(message, attempts, s.registry, operationID)
	latest := make(map[string]domain.DeliveryAttempt, len(providers))
	for _, attempt := range attempts {
		current, ok := latest[attempt.Adapter]
		if !ok || compareAttemptChronology(current, attempt) < 0 {
			latest[attempt.Adapter] = attempt
		}
	}
	eligible := make([]string, 0, len(providers))
	for _, provider := range providers {
		attempt, attempted := latest[provider]
		if !attempted || attempt.Status != domain.AttemptStatusSucceeded {
			eligible = append(eligible, provider)
		}
	}
	return eligible, operationID != uuid.Nil && message.RetryOperationID == operationID, nil
}

func retryPlanProviders(
	message *domain.NotificationMessage,
	attempts []domain.DeliveryAttempt,
	registry *adapters.Registry,
	operationID uuid.UUID,
) []string {
	providers := slices.Clone([]string(message.ProviderPlan))
	if len(providers) == 0 {
		slices.SortFunc(attempts, compareAttemptChronology)
		seen := make(map[string]struct{}, len(attempts))
		for _, attempt := range attempts {
			if _, ok := seen[attempt.Adapter]; ok {
				continue
			}
			seen[attempt.Adapter] = struct{}{}
			providers = append(providers, attempt.Adapter)
		}
	}
	if len(providers) > 0 || (message.Status != domain.MessageStatusFailed &&
		(message.Status != domain.MessageStatusPending || message.RetryOperationID != operationID)) {
		return providers
	}
	for _, candidate := range registry.List(message.Channel) {
		providers = append(providers, candidate.Name())
	}
	return providers
}

func buildDeliveryJobs(
	event *domain.NotificationEvent,
	definition *domain.NotificationDefinition,
	channels []string,
	decision persistencepolicy.Decision,
	existingMessages map[string]*domain.NotificationMessage,
	retryProviders map[string][]string,
	opts DispatchOptions,
) []deliveryJob {
	jobs := make([]deliveryJob, 0, len(channels)*len(event.Recipients))
	for _, channel := range channels {
		templateCode := templateCodeForChannel(definition, channel)
		for _, recipient := range event.Recipients {
			channelType, _ := adapters.ParseChannel(channel)
			key := recipient + "\x00" + channelType
			existing := existingMessages[key]
			providers := retryProviders[key]
			if shouldSkipDelivery(existing, opts.RetryFailedOnly, providers) {
				continue
			}
			locale := opts.Locale
			if opts.RetryFailedOnly && existing != nil && existing.Locale != "" {
				locale = existing.Locale
			}
			if opts.RetryFailedOnly && existing != nil && existing.TemplateCode != "" {
				templateCode = existing.TemplateCode
			}
			jobs = append(jobs, deliveryJob{
				event: event, channel: channel, templateCode: templateCode,
				recipient: recipient, locale: locale, transient: cloneAnyMap(opts.Transient),
				persistence: decision, existing: existing, retryOperationID: opts.RetryOperationID,
				providers: slices.Clone(providers),
			})
		}
	}
	return jobs
}

func shouldSkipDelivery(existing *domain.NotificationMessage, retryFailedOnly bool, providers []string) bool {
	if retryFailedOnly {
		return existing == nil || len(providers) == 0
	}
	return existing != nil && existing.Status == domain.MessageStatusDelivered
}

func (s *Service) executeJobs(
	ctx context.Context,
	event *domain.NotificationEvent,
	definition *domain.NotificationDefinition,
	pending []deliveryJob,
) error {
	jobs := make(chan deliveryJob, len(pending))
	errCh := make(chan error, len(pending))
	var wg sync.WaitGroup
	workerCount := min(s.cfg.MaxWorkers, len(pending))

	for range workerCount {
		wg.Go(func() {
			for job := range jobs {
				if ctx.Err() != nil {
					errCh <- ctx.Err()
					continue
				}
				if err := s.processDelivery(ctx, event, definition, job); err != nil {
					errCh <- err
				}
			}
		})
	}

	for _, job := range pending {
		jobs <- job
	}
	close(jobs)
	wg.Wait()
	close(errCh)

	var deliveryErr error
	for workerErr := range errCh {
		if workerErr != nil {
			deliveryErr = errors.Join(deliveryErr, workerErr)
			safe := s.privacy.SafeError(workerErr)
			s.logger.Error("dispatcher delivery failed", "error_code", safe.Code)
		}
	}
	return deliveryErr
}

func (s *Service) completeDispatch(ctx context.Context, event *domain.NotificationEvent, deliveryErr error) error {
	status := domain.EventStatusProcessed
	if deliveryErr != nil {
		status = domain.EventStatusFailed
	}
	if s.events != nil {
		if err := s.events.UpdateStatus(ctx, event.ID, status); err != nil {
			return fmt.Errorf("dispatcher: update event status: %w", err)
		}
	}
	if deliveryErr != nil {
		return fmt.Errorf("dispatcher: one or more deliveries failed: %w", deliveryErr)
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
		return nil, errors.New("dispatcher: secrets resolver not configured and fallback not allowed")
	}

	refs := []secrets.Reference{
		{Scope: secrets.ScopeUser, SubjectID: job.recipient, Channel: channelType, Provider: provider, Key: "default"},
	}
	if event != nil && strings.TrimSpace(event.TenantID) != "" {
		refs = append(refs, secrets.Reference{Scope: secrets.ScopeTenant, SubjectID: event.TenantID, Channel: channelType, Provider: provider, Key: "default"})
	}
	refs = append(refs, secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: channelType, Provider: provider, Key: "default"})

	resolved, err := s.secrets.Resolve(refs...)
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
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
	return nil, errors.New("dispatcher: no scoped secret and fallback not allowed")
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
	event            *domain.NotificationEvent
	channel          string
	templateCode     string
	recipient        string
	locale           string
	transient        map[string]any
	persistence      persistencepolicy.Decision
	existing         *domain.NotificationMessage
	retryOperationID uuid.UUID
	providers        []string
}

func (s *Service) processDelivery(ctx context.Context, event *domain.NotificationEvent, def *domain.NotificationDefinition, job deliveryJob) error {
	job.persistence = normalizePersistenceDecision(job.persistence)
	state, skipped, err := s.prepareDelivery(ctx, event, def, job)
	if err != nil || skipped {
		return err
	}
	if isInboxChannel(state.channelType) {
		return s.processInboxDelivery(ctx, event, def, job, state)
	}
	return s.processAdapterDelivery(ctx, event, def, job, state)
}

type deliveryState struct {
	channelType       string
	provider          string
	preferredProvider string
	resolvedProvider  string
	renderLocale      string
	attachments       []adapters.Attachment
	renderResult      templates.RenderResult
	message           *domain.NotificationMessage
	candidates        []adapters.Messenger
}

func (s *Service) prepareDelivery(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
) (*deliveryState, bool, error) {
	state, skipped, err := s.initializeDelivery(ctx, event, def, job)
	if err != nil || skipped {
		return nil, skipped, err
	}
	payload, basePayload, attachments := buildDeliveryPayload(event, def, job, state.channelType, state.provider)
	state.attachments = attachments
	if state.preferredProvider != "" {
		state.resolvedProvider = state.preferredProvider
	}
	messageID := uuid.New()
	if job.existing != nil {
		messageID = job.existing.ID
	}
	linkReq, resolvedLinks, attempted, builderOK, err := s.resolveLinks(
		ctx, event, def, job, basePayload, payload, state.channelType,
		state.resolvedProvider, state.renderLocale, messageID,
	)
	if err != nil {
		s.notifyDelivery(ctx, event, def, job, nil, "failed", state.resolvedProvider, state.renderLocale, err)
		return nil, false, err
	}
	applyResolvedLinksToPayload(payload, resolvedLinks)
	if err := s.renderDelivery(ctx, event, def, job, state, payload, messageID); err != nil {
		return nil, false, err
	}
	applyResolvedLinksToMessage(state.message, resolvedLinks)
	if err := s.persistResolvedLinks(ctx, event, def, job, state, linkReq, resolvedLinks, attempted, builderOK); err != nil {
		return nil, false, err
	}
	if err := s.persistInitialMessage(ctx, event, def, job, state); err != nil {
		return nil, false, err
	}
	return state, false, nil
}

func (s *Service) initializeDelivery(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
) (*deliveryState, bool, error) {
	channelType, provider := adapters.ParseChannel(job.channel)
	renderLocale := job.locale
	if renderLocale == "" && event != nil {
		if locale, ok := event.Context["locale"].(string); ok {
			renderLocale = locale
		}
	}
	allowed, reason, providerOverride, err := s.allowDelivery(ctx, event, def, job.recipient, channelType)
	if err != nil {
		return nil, false, fmt.Errorf("preferences evaluation: %w", err)
	}
	if !allowed {
		s.logger.Debug("delivery skipped by preferences", "channel", channelType, "reason", reason)
		return nil, true, nil
	}
	if len(job.providers) > 0 {
		// Retry the durable provider plan. A later preference-provider change
		// may not redirect an explicit retry to a provider that was never part
		// of the original outcome.
		providerOverride = ""
	}
	state := &deliveryState{
		channelType: channelType, provider: provider, preferredProvider: providerOverride,
		resolvedProvider: provider, renderLocale: renderLocale,
	}
	if !isInboxChannel(channelType) {
		routeChannel := job.channel
		if providerOverride != "" {
			routeChannel = fmt.Sprintf("%s:%s", channelType, providerOverride)
		}
		state.candidates = s.registry.List(routeChannel)
		if len(job.providers) > 0 {
			allowedProviders := make(map[string]struct{}, len(job.providers))
			for _, name := range job.providers {
				allowedProviders[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
			}
			state.candidates = slices.DeleteFunc(state.candidates, func(candidate adapters.Messenger) bool {
				_, ok := allowedProviders[strings.ToLower(strings.TrimSpace(candidate.Name()))]
				return !ok
			})
		}
		if len(state.candidates) == 0 {
			return nil, false, fmt.Errorf("route channel %s: %w", routeChannel, adapters.ErrAdapterNotFound)
		}
	}
	return state, false, nil
}

func buildDeliveryPayload(
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
	channelType, provider string,
) (domain.JSONMap, domain.JSONMap, []adapters.Attachment) {
	payload := cloneJSONMap(event.Context)
	if payload == nil {
		payload = make(domain.JSONMap)
	}
	maps.Copy(payload, job.transient)
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
	return payload, basePayload, attachments
}

func (s *Service) renderDelivery(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
	state *deliveryState,
	payload domain.JSONMap,
	messageID uuid.UUID,
) error {
	renderResult, err := s.templates.Render(ctx, templates.RenderRequest{
		Code: job.templateCode, Channel: state.channelType, Locale: state.renderLocale, Data: payload,
	})
	if err != nil {
		s.logger.Error("dispatcher render failed",
			"template", job.templateCode, "channel", state.channelType,
			"definition", def.Code, "event_id", event.ID,
			"error_code", s.privacy.SafeError(err).Code,
		)
		s.notifyDelivery(ctx, event, def, job, nil, "failed", state.provider, state.renderLocale, err)
		return fmt.Errorf("render template %s: %w", job.templateCode, err)
	}
	state.renderResult = renderResult
	state.message = &domain.NotificationMessage{
		RecordMeta: domain.RecordMeta{ID: messageID}, EventID: event.ID,
		RetryOperationID: job.retryOperationID, Channel: state.channelType,
		TemplateCode: job.templateCode,
		Locale:       renderResult.Locale, Subject: renderResult.Subject, Body: renderResult.Body,
		Receiver: job.recipient, Status: domain.MessageStatusPending, Metadata: renderResult.Metadata,
	}
	for _, candidate := range state.candidates {
		state.message.ProviderPlan = append(state.message.ProviderPlan, candidate.Name())
	}
	if job.existing != nil {
		state.message.RecordMeta = job.existing.RecordMeta
		if len(job.existing.ProviderPlan) > 0 {
			state.message.ProviderPlan = slices.Clone(job.existing.ProviderPlan)
		}
	}
	applyChannelOverrides(payload, state.channelType, state.message)
	return nil
}

func (s *Service) persistResolvedLinks(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
	state *deliveryState,
	request links.LinkRequest,
	resolved links.ResolvedLinks,
	attempted, builderOK bool,
) error {
	if !attempted || len(job.transient) > 0 {
		return nil
	}
	if !job.persistence.PersistLinkURLs {
		resolved.ActionURL, resolved.ManifestURL, resolved.URL = "", "", ""
		for idx := range resolved.Records {
			resolved.Records[idx].URL = ""
		}
	}
	if !job.persistence.PersistLinkRecords {
		resolved.Records = nil
	}
	if err := s.invokeLinkHooks(ctx, request, resolved, builderOK && job.persistence.PersistLinkRecords, true); err != nil {
		s.notifyDelivery(ctx, event, def, job, state.message, "failed", state.resolvedProvider, state.renderLocale, err)
		return err
	}
	return nil
}

func (s *Service) persistInitialMessage(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
	state *deliveryState,
) error {
	if s.messages == nil {
		return nil
	}
	if job.existing == nil {
		persistedMessage := projectMessage(state.message, job.persistence)
		if err := s.messages.Create(ctx, persistedMessage); err != nil {
			s.notifyDelivery(ctx, event, def, job, state.message, "failed", state.provider, state.renderLocale, err)
			return fmt.Errorf("persist message: %w", err)
		}
		state.message.RecordMeta = persistedMessage.RecordMeta
		return nil
	}
	if err := s.messages.Update(ctx, projectMessage(state.message, job.persistence)); err != nil {
		return fmt.Errorf("update retry message: %w", err)
	}
	return nil
}

func (s *Service) processInboxDelivery(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
	state *deliveryState,
) error {
	if job.persistence.InboxMode != persistencepolicy.Full {
		state.message.Status = domain.MessageStatusSkipped
		if err := s.updateMessage(ctx, state.message, job.persistence, "skipped"); err != nil {
			return err
		}
		s.notifyDelivery(ctx, event, def, job, state.message, "skipped", state.provider, state.renderLocale, nil)
		return nil
	}
	if s.inbox == nil {
		err := errors.New("dispatcher: inbox channel requested but inbox service is not configured")
		s.notifyDelivery(ctx, event, def, job, state.message, "failed", state.provider, state.renderLocale, err)
		return err
	}
	if err := s.handleInboxDelivery(ctx, state.message); err != nil {
		s.notifyDelivery(ctx, event, def, job, state.message, "failed", state.provider, state.renderLocale, err)
		return err
	}
	if err := s.updateMessage(ctx, state.message, job.persistence, "inbox"); err != nil {
		return err
	}
	s.notifyDelivery(ctx, event, def, job, state.message, "delivered", state.provider, state.renderLocale, nil)
	return nil
}

func (s *Service) processAdapterDelivery(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
	state *deliveryState,
) error {
	var success bool
	var lastErr error
	var lastProvider string

	for _, messenger := range state.candidates {
		if err := s.sendThroughCandidate(ctx, event, def, job, state, messenger); err != nil {
			lastErr = err
			lastProvider = messenger.Name()
			continue
		}
		success = true
		lastProvider = messenger.Name()
	}

	if s.messages != nil {
		if success {
			state.message.Status = domain.MessageStatusDelivered
		} else {
			state.message.Status = domain.MessageStatusFailed
		}
		if err := s.messages.Update(ctx, projectMessage(state.message, job.persistence)); err != nil {
			return errors.Join(lastErr, fmt.Errorf("dispatcher: update message: %w", err))
		}
	}

	if !success {
		s.notifyDelivery(ctx, event, def, job, state.message, "failed", lastProvider, state.renderResult.Locale, lastErr)
		return lastErr
	}
	s.notifyDelivery(ctx, event, def, job, state.message, "delivered", lastProvider, state.renderResult.Locale, nil)
	return nil
}

func (s *Service) sendThroughCandidate(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
	state *deliveryState,
	messenger adapters.Messenger,
) error {
	attachments := state.attachments
	if s.attachments != nil && len(attachments) > 0 {
		resolved, err := s.attachments.Resolve(ctx, adapters.AttachmentJob{
			Channel: state.channelType, Provider: messenger.Name(), Recipient: job.recipient,
			EventID: event.ID.String(), DefinitionCode: def.Code,
		}, attachments)
		if err != nil {
			return err
		}
		attachments = resolved
	}
	secretPayload, err := s.resolveSecrets(ctx, event, job, messenger, state.preferredProvider)
	if err != nil {
		return err
	}
	sendMessage := buildOutboundMessage(event, def, state, messenger, attachments, secretPayload)
	messageCopy := *state.message
	return s.deliverWithRetries(ctx, messenger, &messageCopy, sendMessage, job.persistence)
}

func buildOutboundMessage(
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	state *deliveryState,
	messenger adapters.Messenger,
	attachments []adapters.Attachment,
	secretPayload map[string][]byte,
) adapters.Message {
	metadata := map[string]any{
		"event_id": event.ID.String(), "definition_code": def.Code,
		"correlation_id": event.CorrelationID, "request_id": event.RequestID,
	}
	if len(secretPayload) > 0 {
		metadata["secrets"] = secretPayload
	}
	for key, value := range state.message.Metadata {
		if _, exists := metadata[key]; !exists {
			metadata[key] = value
		}
	}
	return adapters.Message{
		ID: state.message.ID.String(), Channel: state.channelType, Provider: messenger.Name(),
		Subject: state.message.Subject, Body: state.message.Body, To: state.message.Receiver,
		Attachments: attachments, Metadata: metadata, Locale: state.renderResult.Locale,
		TraceID: event.CorrelationID, RequestID: event.RequestID,
	}
}

func (s *Service) updateMessage(
	ctx context.Context,
	message *domain.NotificationMessage,
	decision persistencepolicy.Decision,
	label string,
) error {
	if s.messages == nil {
		return nil
	}
	if err := s.messages.Update(ctx, projectMessage(message, decision)); err != nil {
		return fmt.Errorf("dispatcher: update %s message: %w", label, err)
	}
	return nil
}

func (s *Service) notifyDelivery(
	ctx context.Context,
	event *domain.NotificationEvent,
	def *domain.NotificationDefinition,
	job deliveryJob,
	message *domain.NotificationMessage,
	status, provider, locale string,
	err error,
) {
	s.activity.Notify(ctx, s.buildDeliveryActivity(event, def, job, message, status, provider, locale, err))
}

func normalizePersistenceDecision(decision persistencepolicy.Decision) persistencepolicy.Decision {
	if decision.MessageMode != "" || decision.InboxMode != "" {
		return decision
	}
	decision.MessageMode = persistencepolicy.Full
	decision.InboxMode = persistencepolicy.Full
	decision.PersistLinkURLs = true
	decision.PersistLinkRecords = true
	return decision
}

func (s *Service) deliverWithRetries(ctx context.Context, messenger adapters.Messenger, message *domain.NotificationMessage, sendMsg adapters.Message, decisions ...persistencepolicy.Decision) error {
	decision := persistencepolicy.Decision{
		MessageMode:        persistencepolicy.Full,
		InboxMode:          persistencepolicy.Full,
		PersistLinkURLs:    true,
		PersistLinkRecords: true,
	}
	if len(decisions) > 0 {
		decision = decisions[0]
	}
	var lastErr error
	for attempt := 1; attempt <= s.cfg.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		lastErr = messenger.Send(ctx, sendMsg)
		if lastErr == nil {
			if err := s.recordAttempt(ctx, messenger.Name(), message, domain.AttemptStatusSucceeded, "", attempt); err != nil {
				return fmt.Errorf("dispatcher: record successful delivery attempt: %w", err)
			}
			message.Status = domain.MessageStatusDelivered
			if s.messages != nil {
				if err := s.messages.Update(ctx, projectMessage(message, decision)); err != nil {
					return fmt.Errorf("dispatcher: update delivered message: %w", err)
				}
			}
			return nil
		}
		safe := s.safeError(lastErr)
		s.logger.Warn("delivery error", "attempt", attempt, "error_code", safe.Code)
		if err := s.recordAttempt(ctx, messenger.Name(), message, domain.AttemptStatusFailed, safe.Code, attempt); err != nil {
			return errors.Join(lastErr, fmt.Errorf("dispatcher: record failed delivery attempt: %w", err))
		}
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
		if err := s.messages.Update(ctx, projectMessage(message, decision)); err != nil {
			lastErr = errors.Join(lastErr, fmt.Errorf("dispatcher: update failed message: %w", err))
		}
	}
	return fmt.Errorf("dispatcher: delivery failed after %d attempts: %w", s.cfg.MaxAttempts, lastErr)
}

func (s *Service) recordAttempt(ctx context.Context, adapterName string, message *domain.NotificationMessage, status, errorCode string, attempt int) error {
	if s.attempts == nil {
		return nil
	}
	record := &domain.DeliveryAttempt{
		MessageID:        message.ID,
		RetryOperationID: message.RetryOperationID,
		Adapter:          adapterName,
		Status:           status,
		ErrorCode:        errorCode,
		Payload: domain.JSONMap{
			"attempt": attempt,
		},
	}
	return s.attempts.Create(ctx, record)
}

func (s *Service) safeError(err error) privacy.SafeError {
	if s != nil && s.privacy != nil {
		return s.privacy.SafeError(err)
	}
	return (privacy.DefaultPolicy{}).SafeError(err)
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
	if event != nil {
		meta["correlation_id"] = event.CorrelationID
		meta["request_id"] = event.RequestID
	}
	if err != nil {
		meta["error_code"] = s.privacy.SafeError(err).Code
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
		UserID:         s.privacy.SafeSubjectID(job.recipient),
		TenantID:       tenantID,
		ObjectType:     "notification_message",
		ObjectID:       objectID,
		Channel:        job.channel,
		DefinitionCode: defCode,
		Recipients:     safeRecipients(s.privacy, recipients),
		Metadata:       s.privacy.SafeMetadata(meta),
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
	maps.Copy(dst, src)
	return dst
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	maps.Copy(dst, src)
	return dst
}

func projectMessage(message *domain.NotificationMessage, decision persistencepolicy.Decision) *domain.NotificationMessage {
	if message == nil {
		return nil
	}
	projected := *message
	projected.Metadata = cloneJSONMap(message.Metadata)
	projected.ProviderPlan = slices.Clone(message.ProviderPlan)

	if !decision.PersistLinkURLs {
		clearProjectedContent(&projected, false)
	}

	switch decision.MessageMode {
	case persistencepolicy.Full:
		return &projected
	case persistencepolicy.MetadataOnly:
		clearProjectedContent(&projected, true)
		projected.Metadata = allowedMetadata(projected.Metadata, decision.AllowedMetadata)
	case persistencepolicy.StateOnly:
		clearProjectedContent(&projected, true)
		projected.Metadata = nil
	default:
		// Custom policies must fail closed if they return an unknown mode.
		clearProjectedContent(&projected, true)
		projected.Metadata = nil
	}
	return &projected
}

func clearProjectedContent(message *domain.NotificationMessage, rendered bool) {
	if rendered {
		message.Subject = ""
		message.Body = ""
	}
	message.ActionURL = ""
	message.ManifestURL = ""
	message.URL = ""
}

func allowedMetadata(metadata domain.JSONMap, allowed []string) domain.JSONMap {
	if len(metadata) == 0 || len(allowed) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		key = strings.TrimSpace(key)
		if key != "" {
			allowedSet[key] = struct{}{}
		}
	}
	filtered := make(domain.JSONMap)
	for key, value := range metadata {
		if _, ok := allowedSet[key]; ok && safePersistenceMetadataKey(key) {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func safePersistenceMetadataKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	for _, fragment := range []string{
		"token", "secret", "password", "credential", "authorization",
		"body", "subject", "html", "url", "attachment", "recipient",
		"destination",
	} {
		if key == fragment || strings.Contains(key, fragment) {
			return false
		}
	}
	return true
}

func safeRecipients(policy privacy.Policy, recipients []string) []string {
	if len(recipients) == 0 {
		return nil
	}
	safe := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		if subject := policy.SafeSubjectID(recipient); subject != "" {
			safe = append(safe, subject)
		}
	}
	return safe
}

func (s *Service) reconstructOutcomes(
	ctx context.Context,
	event *domain.NotificationEvent,
	opts DispatchOptions,
	dispatchErr error,
) []receipts.DeliveryOutcome {
	if event == nil {
		return nil
	}
	if s.messages == nil {
		return s.syntheticOutcomes(event, opts, dispatchErr)
	}

	messages, err := s.messages.ListByEvent(ctx, event.ID)
	if err != nil || len(messages) == 0 {
		return s.syntheticOutcomes(event, opts, dispatchErr)
	}
	channels := append([]string(nil), opts.Channels...)
	if len(channels) == 0 {
		channels = append(channels, event.Channels...)
	}
	if len(channels) == 0 {
		if definition, definitionErr := s.definitions.GetByCode(ctx, event.DefinitionCode); definitionErr == nil {
			channels = append(channels, definition.Channels...)
		}
	}
	messageByKey := make(map[string]*domain.NotificationMessage, len(messages))
	for idx := range messages {
		message := &messages[idx]
		messageByKey[message.Receiver+"\x00"+message.Channel] = message
	}
	outcomes := make([]receipts.DeliveryOutcome, 0, len(event.Recipients)*len(channels))
	for _, recipient := range event.Recipients {
		for _, configuredChannel := range channels {
			channel, provider := adapters.ParseChannel(configuredChannel)
			message := messageByKey[recipient+"\x00"+channel]
			if message == nil {
				status := receipts.OutcomeSkipped
				errorCode := ""
				if dispatchErr != nil {
					status = receipts.OutcomeFailed
					errorCode = s.privacy.SafeError(dispatchErr).Code
				}
				outcomes = append(outcomes, receipts.DeliveryOutcome{
					SubjectID: s.privacy.SafeSubjectID(recipient), Channel: channel,
					Provider: provider, Status: status, ErrorCode: errorCode,
				})
				continue
			}
			outcomes = append(outcomes, s.messageOutcomes(ctx, message, s.registry.List(configuredChannel))...)
		}
	}
	return outcomes
}

type providerAttemptResult struct {
	last  domain.DeliveryAttempt
	count int
}

func (s *Service) messageOutcomes(ctx context.Context, message *domain.NotificationMessage, candidates []adapters.Messenger) []receipts.DeliveryOutcome {
	base := receipts.DeliveryOutcome{
		MessageID: message.ID,
		SubjectID: s.privacy.SafeSubjectID(message.Receiver),
		Channel:   message.Channel,
		Status:    receiptOutcomeStatus(message.Status),
	}
	if s.attempts == nil {
		return []receipts.DeliveryOutcome{base}
	}
	attempts, err := s.attempts.ListByMessage(ctx, message.ID)
	if err != nil || (len(attempts) == 0 && len(candidates) == 0) {
		return []receipts.DeliveryOutcome{base}
	}
	slices.SortFunc(attempts, compareAttempts)

	byProvider := make(map[string]providerAttemptResult)
	for _, attempt := range attempts {
		result := byProvider[attempt.Adapter]
		result.last = attempt
		result.count++
		byProvider[attempt.Adapter] = result
	}
	providers := orderedOutcomeProviders(message.ProviderPlan, candidates, byProvider)
	outcomes := make([]receipts.DeliveryOutcome, 0, len(providers))
	for _, provider := range providers {
		result, attempted := byProvider[provider]
		status := receipts.OutcomeFailed
		if attempted && result.last.Status == domain.AttemptStatusSucceeded {
			status = receipts.OutcomeDelivered
		}
		outcomes = append(outcomes, receipts.DeliveryOutcome{
			MessageID:    message.ID,
			SubjectID:    base.SubjectID,
			Channel:      message.Channel,
			Provider:     provider,
			Status:       status,
			AttemptCount: result.count,
			ErrorCode:    result.last.ErrorCode,
		})
	}
	return outcomes
}

func orderedOutcomeProviders(
	providerPlan domain.StringList,
	candidates []adapters.Messenger,
	byProvider map[string]providerAttemptResult,
) []string {
	providers := make([]string, 0, len(byProvider)+len(providerPlan))
	seen := make(map[string]struct{}, len(byProvider)+len(providerPlan))
	for _, provider := range providerPlan {
		if _, ok := seen[provider]; !ok {
			providers = append(providers, provider)
			seen[provider] = struct{}{}
		}
	}
	if len(providerPlan) == 0 {
		for _, candidate := range candidates {
			if _, ok := seen[candidate.Name()]; !ok {
				providers = append(providers, candidate.Name())
				seen[candidate.Name()] = struct{}{}
			}
		}
	}
	extras := make([]string, 0, len(byProvider))
	for provider := range byProvider {
		if _, ok := seen[provider]; !ok {
			extras = append(extras, provider)
		}
	}
	slices.Sort(extras)
	providers = append(providers, extras...)
	return providers
}

func compareAttempts(left, right domain.DeliveryAttempt) int {
	if cmp := strings.Compare(left.Adapter, right.Adapter); cmp != 0 {
		return cmp
	}
	if left.CreatedAt.Before(right.CreatedAt) {
		return -1
	}
	if left.CreatedAt.After(right.CreatedAt) {
		return 1
	}
	return strings.Compare(left.ID.String(), right.ID.String())
}

func compareAttemptChronology(left, right domain.DeliveryAttempt) int {
	if left.CreatedAt.Before(right.CreatedAt) {
		return -1
	}
	if left.CreatedAt.After(right.CreatedAt) {
		return 1
	}
	return strings.Compare(left.ID.String(), right.ID.String())
}

func (s *Service) syntheticOutcomes(
	event *domain.NotificationEvent,
	opts DispatchOptions,
	dispatchErr error,
) []receipts.DeliveryOutcome {
	channels := append([]string(nil), opts.Channels...)
	if len(channels) == 0 {
		if definition, err := s.definitions.GetByCode(context.Background(), event.DefinitionCode); err == nil {
			channels = append(channels, definition.Channels...)
		}
	}
	if len(channels) == 0 {
		channels = []string{""}
	}
	status := receipts.OutcomePending
	errorCode := ""
	if dispatchErr != nil {
		status = receipts.OutcomeFailed
		errorCode = s.privacy.SafeError(dispatchErr).Code
	}
	outcomes := make([]receipts.DeliveryOutcome, 0, len(event.Recipients)*len(channels))
	for _, recipient := range event.Recipients {
		for _, channel := range channels {
			outcomes = append(outcomes, receipts.DeliveryOutcome{
				SubjectID: s.privacy.SafeSubjectID(recipient),
				Channel:   channel,
				Status:    status,
				ErrorCode: errorCode,
			})
		}
	}
	return outcomes
}

func receiptOutcomeStatus(status string) receipts.OutcomeStatus {
	switch status {
	case domain.MessageStatusDelivered:
		return receipts.OutcomeDelivered
	case domain.MessageStatusFailed:
		return receipts.OutcomeFailed
	case domain.MessageStatusSkipped:
		return receipts.OutcomeSkipped
	default:
		return receipts.OutcomePending
	}
}

func aggregateReceiptStatus(outcomes []receipts.DeliveryOutcome, dispatchErr error) receipts.Status {
	if len(outcomes) == 0 {
		if dispatchErr != nil {
			return receipts.StatusFailed
		}
		return receipts.StatusProcessed
	}
	delivered := 0
	failed := 0
	pending := 0
	for _, outcome := range outcomes {
		switch outcome.Status {
		case receipts.OutcomeDelivered:
			delivered++
		case receipts.OutcomeFailed:
			failed++
		case receipts.OutcomePending:
			pending++
		}
	}
	switch {
	case pending > 0 && delivered == 0 && failed == 0:
		return receipts.StatusDispatching
	case delivered > 0 && failed > 0:
		return receipts.StatusPartial
	case failed > 0 || dispatchErr != nil:
		return receipts.StatusFailed
	default:
		return receipts.StatusProcessed
	}
}

func receiptEventStatus(status receipts.Status) string {
	switch status {
	case receipts.StatusProcessed:
		return domain.EventStatusProcessed
	case receipts.StatusPartial:
		return domain.EventStatusPartial
	case receipts.StatusFailed:
		return domain.EventStatusFailed
	default:
		return ""
	}
}

func receiptStatusForEvent(eventStatus string, outcomes []receipts.DeliveryOutcome) receipts.Status {
	switch eventStatus {
	case domain.EventStatusScheduled:
		return receipts.StatusScheduled
	case domain.EventStatusDispatching:
		return receipts.StatusDispatching
	case domain.EventStatusRetrying:
		return receipts.StatusRetrying
	case domain.EventStatusProcessed, domain.EventStatusPartial, domain.EventStatusFailed:
		status := aggregateReceiptStatus(outcomes, nil)
		if status == receipts.StatusDispatching {
			switch eventStatus {
			case domain.EventStatusProcessed:
				return receipts.StatusProcessed
			case domain.EventStatusPartial:
				return receipts.StatusPartial
			default:
				return receipts.StatusFailed
			}
		}
		return status
	default:
		return receipts.StatusAccepted
	}
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
	if subject := firstString(overrides, "subject"); subject != "" {
		message.Subject = subject
	}
	if body := firstString(overrides, "body"); body != "" {
		message.Body = body
	}
	for _, key := range []string{"html_body", "text_body", "icon", "badge", "cta_label"} {
		if value := firstString(overrides, key); value != "" {
			message.Metadata[key] = value
		}
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

func linkMetadataFromPayload(payload domain.JSONMap, channel string) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	var metadata map[string]any
	if raw, ok := payload["metadata"]; ok {
		switch typed := raw.(type) {
		case map[string]any:
			metadata = cloneAnyMap(typed)
		case domain.JSONMap:
			metadata = cloneAnyMap(typed)
		}
	}
	overrides := extractOverrides(payload, channel)
	for _, key := range []string{"html_body", "text_body", "icon", "badge", "cta_label"} {
		if value := firstString(overrides, key); value != "" {
			if metadata == nil {
				metadata = make(map[string]any)
			}
			metadata[key] = value
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func (s *Service) resolveLinks(ctx context.Context, event *domain.NotificationEvent, def *domain.NotificationDefinition, job deliveryJob, basePayload, payload domain.JSONMap, channel, provider, locale string, messageID uuid.UUID) (links.LinkRequest, links.ResolvedLinks, bool, bool, error) {
	// Precedence: overrides > original (builder wins later).
	baseResolved := mergeResolvedLinks(
		resolvedLinksFromPayload(basePayload),
		resolvedLinksFromOverrides(basePayload, channel),
	)
	baseResolved = normalizeResolvedLinks(baseResolved)
	if s.linkBuilder == nil {
		return links.LinkRequest{}, baseResolved, false, false, nil
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
		Metadata:     linkMetadataFromPayload(payload, channel),
		ResolvedURLs: resolvedURLsFromPayload(payload),
	}
	resolved, err := s.linkBuilder.Build(ctx, req)
	if err != nil {
		if s.linkPolicy.Builder == links.FailureLenient {
			s.logger.Warn("link builder failed; continuing with pass-through links",
				"definition", def.Code,
				"channel", job.channel,
				"error_code", s.privacy.SafeError(err).Code,
			)
			baseResolved = ensureResolvedLinkRecords(req, baseResolved)
			return req, baseResolved, true, false, nil
		}
		return req, links.ResolvedLinks{}, true, false, err
	}
	resolved = normalizeResolvedLinks(resolved)
	merged := mergeResolvedLinks(baseResolved, resolved)
	merged = normalizeResolvedLinks(merged)
	merged = ensureResolvedLinkRecords(req, merged)
	return req, merged, true, true, nil
}

func (s *Service) invokeLinkHooks(ctx context.Context, req links.LinkRequest, resolved links.ResolvedLinks, allowStore, allowObserver bool) error {
	req.Recipient = s.privacy.SafeSubjectID(req.Recipient)
	req.Payload = s.privacy.SafeContext(req.Payload)
	req.Metadata = s.privacy.SafeMetadata(req.Metadata)
	req.ResolvedURLs = nil
	resolved.Metadata = s.privacy.SafeMetadata(resolved.Metadata)
	for idx := range resolved.Records {
		resolved.Records[idx].Recipient = s.privacy.SafeSubjectID(resolved.Records[idx].Recipient)
		resolved.Records[idx].Metadata = s.privacy.SafeMetadata(resolved.Records[idx].Metadata)
	}
	if allowStore && s.linkStore != nil && len(resolved.Records) > 0 {
		if err := s.linkStore.Save(ctx, resolved.Records); err != nil {
			if s.linkPolicy.Store == links.FailureLenient {
				s.logger.Warn("link store save failed; continuing",
					"definition", req.Definition,
					"channel", req.Channel,
					"error_code", s.privacy.SafeError(err).Code,
				)
			} else {
				return err
			}
		}
	}
	if allowObserver && s.linkObserver != nil {
		observerResolved := resolved
		observerResolved.ActionURL = ""
		observerResolved.ManifestURL = ""
		observerResolved.URL = ""
		observerResolved.Records = append([]links.LinkRecord(nil), resolved.Records...)
		for idx := range observerResolved.Records {
			observerResolved.Records[idx].URL = ""
		}
		if err := s.notifyLinkObserver(ctx, req, observerResolved); err != nil {
			if s.linkPolicy.Observer == links.FailureLenient {
				s.logger.Warn("link observer failed; continuing",
					"definition", req.Definition,
					"channel", req.Channel,
					"error_code", s.privacy.SafeError(err).Code,
				)
			} else {
				return err
			}
		}
	}
	return nil
}

func (s *Service) notifyLinkObserver(ctx context.Context, req links.LinkRequest, resolved links.ResolvedLinks) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("link observer panic: %v", recovered)
		}
	}()
	s.linkObserver.OnLinksResolved(ctx, links.LinkResolution{
		Request:  req,
		Resolved: resolved,
	})
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
		maps.Copy(base.Metadata, override.Metadata)
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
		maps.Copy(message.Metadata, resolved.Metadata)
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

func isInboxChannel(channel string) bool {
	switch channel {
	case "inbox", "in-app", "inapp", "in_app":
		return true
	default:
		return false
	}
}
