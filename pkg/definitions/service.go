package definitions

import (
	"context"
	"errors"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
)

const CommandTypeUpsert = "notifications.definition.upsert"

// UpsertInput is the serializable definition mutation payload.
type UpsertInput struct {
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Severity    string         `json:"severity"`
	Category    string         `json:"category"`
	Channels    []string       `json:"channels"`
	TemplateIDs []string       `json:"template_keys"`
	Metadata    map[string]any `json:"metadata"`
	AllowUpdate bool           `json:"allow_update"`
	Policy      map[string]any `json:"policy"`
}

func (UpsertInput) Type() string { return CommandTypeUpsert }

func (input UpsertInput) Validate() error {
	if strings.TrimSpace(input.Code) == "" {
		return errors.New("definitions: code is required")
	}
	return nil
}

// Service owns definition mutation orchestration.
type Service struct {
	repository store.NotificationDefinitionRepository
}

func New(repository store.NotificationDefinitionRepository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("definitions: repository is required")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Upsert(ctx context.Context, input UpsertInput) (*domain.NotificationDefinition, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("definitions: service is not initialized")
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	code := strings.TrimSpace(input.Code)
	record := &domain.NotificationDefinition{
		Code:         code,
		Name:         strings.TrimSpace(input.Name),
		Description:  strings.TrimSpace(input.Description),
		Severity:     strings.TrimSpace(input.Severity),
		Category:     strings.TrimSpace(input.Category),
		Channels:     domain.StringList(input.Channels),
		TemplateKeys: domain.StringList(input.TemplateIDs),
		Metadata:     domain.JSONMap(input.Metadata),
		Policy:       domain.JSONMap(input.Policy),
	}
	existing, err := s.repository.GetByCode(ctx, code)
	switch {
	case err == nil:
		if !input.AllowUpdate {
			return nil, errors.New("definitions: definition already exists")
		}
		existing.Name = record.Name
		existing.Description = record.Description
		existing.Severity = record.Severity
		existing.Category = record.Category
		existing.Channels = record.Channels
		existing.TemplateKeys = record.TemplateKeys
		existing.Metadata = record.Metadata
		existing.Policy = record.Policy
		if updateErr := s.repository.Update(ctx, existing); updateErr != nil {
			return nil, updateErr
		}
		return existing, nil
	case errors.Is(err, store.ErrNotFound):
		if createErr := s.repository.Create(ctx, record); createErr != nil {
			return nil, createErr
		}
		return record, nil
	default:
		return nil, err
	}
}
