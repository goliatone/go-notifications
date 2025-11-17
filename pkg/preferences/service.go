package preferences

import (
	"context"
	"errors"

	internalprefs "github.com/goliatone/go-notifications/internal/preferences"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	opts "github.com/goliatone/go-options"
)

// Re-export types required by consumers so they do not depend on the internal package.
type (
	PreferenceInput   = internalprefs.PreferenceInput
	EvaluationRequest = internalprefs.EvaluationRequest
	EvaluationResult  = internalprefs.EvaluationResult
	QuietHoursWindow  = internalprefs.QuietHoursWindow
)

const (
	ReasonDefault            = internalprefs.ReasonDefault
	ReasonOptOut             = internalprefs.ReasonOptOut
	ReasonQuietHours         = internalprefs.ReasonQuietHours
	ReasonChannelOverride    = internalprefs.ReasonChannelOverride
	ReasonSubscriptionFilter = internalprefs.ReasonSubscriptionFilter
)

// Service exposes CRUD and evaluation helpers to consumers.
type Service struct {
	internal *internalprefs.Service
}

// Dependencies wires repositories and loggers into the service.
type Dependencies struct {
	Repository store.NotificationPreferenceRepository
	Logger     logger.Logger
}

var errServiceNotInitialised = errors.New("preferences: service not initialised")

// New constructs the preferences facade backed by the internal service.
func New(deps Dependencies) (*Service, error) {
	internal, err := internalprefs.NewService(internalprefs.Dependencies{
		Repository: deps.Repository,
		Logger:     deps.Logger,
	})
	if err != nil {
		return nil, err
	}
	return &Service{internal: internal}, nil
}

// Create persists a preference record.
func (s *Service) Create(ctx context.Context, input PreferenceInput) (*domain.NotificationPreference, error) {
	if s == nil || s.internal == nil {
		return nil, errServiceNotInitialised
	}
	return s.internal.Create(ctx, input)
}

// Update mutates an existing preference record.
func (s *Service) Update(ctx context.Context, input PreferenceInput) (*domain.NotificationPreference, error) {
	if s == nil || s.internal == nil {
		return nil, errServiceNotInitialised
	}
	return s.internal.Update(ctx, input)
}

// Upsert creates or updates a preference record.
func (s *Service) Upsert(ctx context.Context, input PreferenceInput) (*domain.NotificationPreference, error) {
	if s == nil || s.internal == nil {
		return nil, errServiceNotInitialised
	}
	return s.internal.Upsert(ctx, input)
}

// Delete removes the preference record for the provided subject.
func (s *Service) Delete(ctx context.Context, subjectType, subjectID, definitionCode, channel string) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	return s.internal.Delete(ctx, subjectType, subjectID, definitionCode, channel)
}

// Get fetches the stored preference record for the subject.
func (s *Service) Get(ctx context.Context, subjectType, subjectID, definitionCode, channel string) (*domain.NotificationPreference, error) {
	if s == nil || s.internal == nil {
		return nil, errServiceNotInitialised
	}
	return s.internal.Get(ctx, subjectType, subjectID, definitionCode, channel)
}

// List enumerates stored preferences using repository pagination.
func (s *Service) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationPreference], error) {
	if s == nil || s.internal == nil {
		return store.ListResult[domain.NotificationPreference]{}, errServiceNotInitialised
	}
	return s.internal.List(ctx, opts)
}

// Evaluate resolves scoped preferences and returns the enforcement result.
func (s *Service) Evaluate(ctx context.Context, req EvaluationRequest) (EvaluationResult, error) {
	if s == nil || s.internal == nil {
		return EvaluationResult{}, errServiceNotInitialised
	}
	return s.internal.Evaluate(ctx, req)
}

// ResolveWithTrace evaluates the request and resolves the provided path.
func (s *Service) ResolveWithTrace(ctx context.Context, req EvaluationRequest, path string) (any, opts.Trace, error) {
	result, err := s.Evaluate(ctx, req)
	if err != nil {
		return nil, opts.Trace{Path: path}, err
	}
	if result.Resolver == nil {
		return nil, opts.Trace{Path: path}, errors.New("preferences: resolver unavailable")
	}
	return result.Resolver.Resolve(path)
}

// Schema evaluates the request and exports the schema document for UI tooling.
func (s *Service) Schema(ctx context.Context, req EvaluationRequest) (opts.SchemaDocument, error) {
	result, err := s.Evaluate(ctx, req)
	if err != nil {
		return opts.SchemaDocument{}, err
	}
	if result.Resolver == nil {
		return opts.SchemaDocument{}, errors.New("preferences: resolver unavailable")
	}
	return result.Resolver.Schema()
}
