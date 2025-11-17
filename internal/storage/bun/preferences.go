package bunrepo

import (
	"context"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type PreferenceRepository struct {
	base baseRepository[domain.NotificationPreference]
}

func NewPreferenceRepository(db *bun.DB) *PreferenceRepository {
	handlers := repository.ModelHandlers[*domain.NotificationPreference]{
		NewRecord:          func() *domain.NotificationPreference { return &domain.NotificationPreference{} },
		GetID:              func(p *domain.NotificationPreference) uuid.UUID { return p.ID },
		SetID:              func(p *domain.NotificationPreference, id uuid.UUID) { p.ID = id },
		GetIdentifier:      func() string { return "id" },
		GetIdentifierValue: func(p *domain.NotificationPreference) string { return p.ID.String() },
	}
	return &PreferenceRepository{
		base: newBaseRepository[domain.NotificationPreference](db, handlers, func(p *domain.NotificationPreference) *domain.RecordMeta { return &p.RecordMeta }),
	}
}

func (r *PreferenceRepository) Create(ctx context.Context, pref *domain.NotificationPreference) error {
	return r.base.create(ctx, pref)
}

func (r *PreferenceRepository) Update(ctx context.Context, pref *domain.NotificationPreference) error {
	return r.base.update(ctx, pref)
}

func (r *PreferenceRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationPreference, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *PreferenceRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationPreference], error) {
	return r.base.list(ctx, opts)
}

func (r *PreferenceRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *PreferenceRepository) GetBySubject(ctx context.Context, subjectType, subjectID, definitionCode, channel string) (*domain.NotificationPreference, error) {
	record, err := r.base.repo.Get(ctx,
		func(q *bun.SelectQuery) *bun.SelectQuery {
			q = q.Where("LOWER(subject_type) = ?", strings.ToLower(subjectType)).
				Where("subject_id = ?", subjectID)
			if definitionCode != "" {
				q = q.Where("LOWER(definition_code) = ?", strings.ToLower(definitionCode))
			}
			if channel != "" {
				q = q.Where("LOWER(channel) = ?", strings.ToLower(channel))
			}
			return q
		},
		withoutDeleted(),
	)
	if err != nil {
		return nil, mapError(err)
	}
	return record, nil
}
