package preferences

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	pkgoptions "github.com/goliatone/go-notifications/pkg/options"
	opts "github.com/goliatone/go-options"
)

// Reason identifiers used during evaluation.
const (
	ReasonDefault            = "default"
	ReasonOptOut             = "opt-out"
	ReasonQuietHours         = "quiet-hours"
	ReasonChannelOverride    = "channel-override"
	ReasonSubscriptionFilter = "subscription-filter"
)

// QuietHoursWindow models a quiet hours schedule relative to a timezone.
type QuietHoursWindow struct {
	Start    string
	End      string
	Timezone string
}

// PreferenceInput captures persistence fields for CRUD operations.
type PreferenceInput struct {
	SubjectType    string            `json:"subject_type"`
	SubjectID      string            `json:"subject_id"`
	DefinitionCode string            `json:"definition_code"`
	Channel        string            `json:"channel"`
	Enabled        *bool             `json:"enabled,omitempty"`
	Locale         *string           `json:"locale,omitempty"`
	QuietHours     *QuietHoursWindow `json:"quiet_hours,omitempty"`
	Provider       *string           `json:"provider,omitempty"`
	Rules          domain.JSONMap    `json:"rules,omitempty"`
}

// EvaluationRequest defines the scoped lookup performed before dispatch.
type EvaluationRequest struct {
	DefinitionCode string
	Channel        string
	Scopes         []pkgoptions.PreferenceScopeRef
	Subscriptions  []string
	Timestamp      time.Time
	DefaultEnabled *bool
}

// EvaluationResult returns the computed state along with traces.
type EvaluationResult struct {
	Allowed           bool
	Reason            string
	QuietHoursActive  bool
	ChannelOverride   bool
	Provider          string
	Trace             opts.Trace
	ChannelTrace      opts.Trace
	ProviderTrace     opts.Trace
	Resolver          *pkgoptions.Resolver
	RequiredSubs      []string
	SubscriptionTrace opts.Trace
}

// Dependencies wires repositories and logging into the service.
type Dependencies struct {
	Repository store.NotificationPreferenceRepository
	Logger     logger.Logger
	Clock      func() time.Time
}

// Service persists preferences and evaluates scope-aware rules.
type Service struct {
	repo  store.NotificationPreferenceRepository
	log   logger.Logger
	clock func() time.Time
}

var (
	errRepositoryRequired = errors.New("preferences: repository is required")
)

// NewService constructs the preferences service.
func NewService(deps Dependencies) (*Service, error) {
	if deps.Repository == nil {
		return nil, errRepositoryRequired
	}
	if deps.Logger == nil {
		deps.Logger = logger.Default()
	}
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	return &Service{
		repo:  deps.Repository,
		log:   deps.Logger,
		clock: deps.Clock,
	}, nil
}

// Create persists a brand new preference record.
func (s *Service) Create(ctx context.Context, input PreferenceInput) (*domain.NotificationPreference, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetBySubject(ctx, input.SubjectType, input.SubjectID, input.DefinitionCode, input.Channel); err == nil {
		return nil, fmt.Errorf("preferences: record already exists for %s/%s", input.SubjectType, input.SubjectID)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	record := newPreferenceRecord(input)
	if err := s.repo.Create(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

// Update mutates an existing preference record.
func (s *Service) Update(ctx context.Context, input PreferenceInput) (*domain.NotificationPreference, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	current, err := s.repo.GetBySubject(ctx, input.SubjectType, input.SubjectID, input.DefinitionCode, input.Channel)
	if err != nil {
		return nil, err
	}
	applyInput(current, input)
	if err := s.repo.Update(ctx, current); err != nil {
		return nil, err
	}
	return current, nil
}

// Upsert creates or updates a preference record.
func (s *Service) Upsert(ctx context.Context, input PreferenceInput) (*domain.NotificationPreference, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	current, err := s.repo.GetBySubject(ctx, input.SubjectType, input.SubjectID, input.DefinitionCode, input.Channel)
	switch {
	case err == nil:
		applyInput(current, input)
		if err := s.repo.Update(ctx, current); err != nil {
			return nil, err
		}
		return current, nil
	case errors.Is(err, store.ErrNotFound):
		record := newPreferenceRecord(input)
		if err := s.repo.Create(ctx, record); err != nil {
			return nil, err
		}
		return record, nil
	default:
		return nil, err
	}
}

// Delete soft deletes the preference record for the provided subject.
func (s *Service) Delete(ctx context.Context, subjectType, subjectID, definitionCode, channel string) error {
	record, err := s.repo.GetBySubject(ctx, subjectType, subjectID, definitionCode, channel)
	if err != nil {
		return err
	}
	return s.repo.SoftDelete(ctx, record.ID)
}

// Get fetches the stored preference for a subject.
func (s *Service) Get(ctx context.Context, subjectType, subjectID, definitionCode, channel string) (*domain.NotificationPreference, error) {
	return s.repo.GetBySubject(ctx, subjectType, subjectID, definitionCode, channel)
}

// List enumerates preference records using repository pagination.
func (s *Service) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationPreference], error) {
	return s.repo.List(ctx, opts)
}

// Evaluate merges scope snapshots and enforces opt-out rules prior to dispatch.
func (s *Service) Evaluate(ctx context.Context, req EvaluationRequest) (EvaluationResult, error) {
	result := EvaluationResult{
		Allowed: true,
		Reason:  ReasonDefault,
	}
	if len(req.Scopes) == 0 {
		return result, errors.New("preferences: at least one scope is required")
	}
	if strings.TrimSpace(req.DefinitionCode) == "" {
		return result, errors.New("preferences: definition code is required")
	}
	if strings.TrimSpace(req.Channel) == "" {
		return result, errors.New("preferences: channel is required")
	}

	refScopes := normalizeScopes(req)
	store := pkgoptions.PreferenceSnapshotStore{Repository: s.repo}
	snapshots, err := store.Load(ctx, refScopes)
	if err != nil {
		return result, err
	}

	defaultState := true
	if req.DefaultEnabled != nil {
		defaultState = *req.DefaultEnabled
	}
	snapshots = append(snapshots, pkgoptions.Snapshot{
		Scope: opts.NewScope("defaults", opts.ScopePrioritySystem-1000, opts.WithScopeLabel("Defaults")),
		Data: map[string]any{
			"enabled": defaultState,
		},
	})

	resolver, err := pkgoptions.NewResolver(snapshots...)
	if err != nil {
		return result, err
	}
	result.Resolver = resolver

	if enabled, trace, err := resolver.ResolveBool("enabled"); err == nil {
		result.Allowed = enabled
		result.Trace = trace
		if !enabled {
			result.Reason = ReasonOptOut
		}
	} else {
		result.Allowed = defaultState
	}

	if req.Channel != "" {
		channelPath := fmt.Sprintf("rules.channels.%s.enabled", strings.ToLower(req.Channel))
		if channelState, trace, err := resolver.ResolveBool(channelPath); err == nil {
			result.ChannelOverride = true
			result.ChannelTrace = trace
			if !channelState {
				if result.Allowed || result.Reason == ReasonDefault {
					result.Reason = ReasonChannelOverride
				}
				result.Allowed = false
			}
		}
		// Provider override at channel level
		if provider, trace, err := resolver.ResolveString(fmt.Sprintf("rules.channels.%s.provider", strings.ToLower(req.Channel))); err == nil && strings.TrimSpace(provider) != "" {
			result.Provider = strings.TrimSpace(provider)
			result.ProviderTrace = trace
		}
	}
	// Fallback provider at root rules if channel-specific not set.
	if result.Provider == "" {
		if provider, trace, err := resolver.ResolveString("rules.provider"); err == nil && strings.TrimSpace(provider) != "" {
			result.Provider = strings.TrimSpace(provider)
			result.ProviderTrace = trace
		}
	}

	if window, ok := resolveQuietHours(resolver); ok {
		ts := req.Timestamp
		if ts.IsZero() {
			ts = s.clock()
		}
		if window.contains(ts) {
			if result.Allowed || result.Reason == ReasonDefault {
				result.Reason = ReasonQuietHours
			}
			result.Allowed = false
			result.QuietHoursActive = true
		}
	}

	if subs, trace, err := resolver.ResolveStringSlice("rules.subscriptions"); err == nil && len(subs) > 0 {
		result.RequiredSubs = subs
		result.SubscriptionTrace = trace
		if !intersects(subs, req.Subscriptions) {
			if result.Allowed || result.Reason == ReasonDefault {
				result.Reason = ReasonSubscriptionFilter
			}
			result.Allowed = false
		}
	}

	return result, nil
}

func normalizeScopes(req EvaluationRequest) []pkgoptions.PreferenceScopeRef {
	out := make([]pkgoptions.PreferenceScopeRef, len(req.Scopes))
	for i, scope := range req.Scopes {
		scope.DefinitionCode = fallback(scope.DefinitionCode, req.DefinitionCode)
		scope.Channel = fallback(scope.Channel, req.Channel)
		out[i] = scope
	}
	return out
}

func fallback(value, defaultVal string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return defaultVal
}

func newPreferenceRecord(input PreferenceInput) *domain.NotificationPreference {
	record := &domain.NotificationPreference{
		SubjectType:    strings.TrimSpace(strings.ToLower(input.SubjectType)),
		SubjectID:      strings.TrimSpace(input.SubjectID),
		DefinitionCode: strings.TrimSpace(input.DefinitionCode),
		Channel:        strings.TrimSpace(input.Channel),
		Enabled:        true,
	}
	applyInput(record, input)
	if record.Locale == "" && input.Locale != nil {
		record.Locale = strings.TrimSpace(*input.Locale)
	}
	if input.Enabled != nil {
		record.Enabled = *input.Enabled
	}
	return record
}

func applyInput(record *domain.NotificationPreference, input PreferenceInput) {
	if record == nil {
		return
	}
	if record.AdditionalRules == nil {
		record.AdditionalRules = make(domain.JSONMap)
	}
	if input.Enabled != nil {
		record.Enabled = *input.Enabled
	}
	if input.Locale != nil {
		record.Locale = strings.TrimSpace(*input.Locale)
	}
	if quietMap, ok := quietHoursToJSON(input.QuietHours); ok {
		record.QuietHours = quietMap
	}
	if input.Rules != nil {
		record.AdditionalRules = copyJSONMap(input.Rules)
	}
	if input.Provider != nil {
		record.AdditionalRules["provider"] = strings.TrimSpace(*input.Provider)
	}
}

func quietHoursToJSON(window *QuietHoursWindow) (domain.JSONMap, bool) {
	if window == nil {
		return nil, false
	}
	start := strings.TrimSpace(window.Start)
	end := strings.TrimSpace(window.End)
	if start == "" && end == "" {
		return nil, true
	}
	result := domain.JSONMap{
		"start": start,
		"end":   end,
	}
	if tz := strings.TrimSpace(window.Timezone); tz != "" {
		result["timezone"] = tz
	}
	return result, true
}

func copyJSONMap(src domain.JSONMap) domain.JSONMap {
	if len(src) == 0 {
		return nil
	}
	dst := make(domain.JSONMap, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func validateInput(input PreferenceInput) error {
	if strings.TrimSpace(input.SubjectType) == "" {
		return errors.New("preferences: subject type is required")
	}
	if strings.TrimSpace(input.SubjectID) == "" {
		return errors.New("preferences: subject id is required")
	}
	if strings.TrimSpace(input.DefinitionCode) == "" {
		return errors.New("preferences: definition code is required")
	}
	if strings.TrimSpace(input.Channel) == "" {
		return errors.New("preferences: channel is required")
	}
	return nil
}

type quietHours struct {
	start    string
	end      string
	timezone string
}

func resolveQuietHours(resolver *pkgoptions.Resolver) (quietHours, bool) {
	if resolver == nil {
		return quietHours{}, false
	}
	value, _, err := resolver.Resolve("quiet_hours")
	if err != nil {
		return quietHours{}, false
	}
	switch v := value.(type) {
	case map[string]any:
		return quietHours{
			start:    asString(v["start"]),
			end:      asString(v["end"]),
			timezone: asString(v["timezone"]),
		}, true
	case domain.JSONMap:
		return quietHours{
			start:    asString(v["start"]),
			end:      asString(v["end"]),
			timezone: asString(v["timezone"]),
		}, true
	default:
		return quietHours{}, false
	}
}

func (q quietHours) contains(ts time.Time) bool {
	if q.start == "" || q.end == "" {
		return false
	}
	loc := time.UTC
	if q.timezone != "" {
		if location, err := time.LoadLocation(q.timezone); err == nil {
			loc = location
		}
	}
	now := ts.In(loc)
	startClock, err := time.Parse("15:04", q.start)
	if err != nil {
		return false
	}
	endClock, err := time.Parse("15:04", q.end)
	if err != nil {
		return false
	}

	start := time.Date(now.Year(), now.Month(), now.Day(), startClock.Hour(), startClock.Minute(), 0, 0, loc)
	end := time.Date(now.Year(), now.Month(), now.Day(), endClock.Hour(), endClock.Minute(), 0, 0, loc)

	if !end.After(start) {
		// Wrap around midnight.
		end = end.Add(24 * time.Hour)
		if now.Before(start) {
			start = start.Add(-24 * time.Hour)
		}
	}
	if now.Before(start) || !now.Before(end) {
		return false
	}
	return true
}

func intersects(allowed, provided []string) bool {
	if len(allowed) == 0 {
		return true
	}
	if len(provided) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(provided))
	for _, entry := range provided {
		set[strings.ToLower(strings.TrimSpace(entry))] = struct{}{}
	}
	for _, entry := range allowed {
		if _, ok := set[strings.ToLower(strings.TrimSpace(entry))]; ok {
			return true
		}
	}
	return false
}

func asString(value any) string {
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return ""
}
