package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type PreferenceRepository struct {
	base      baseMemoryRepo[domain.NotificationPreference]
	bySubject map[string]uuid.UUID
}

func NewPreferenceRepository() *PreferenceRepository {
	return &PreferenceRepository{
		base:      newBaseMemoryRepo("preference", func(p *domain.NotificationPreference) *domain.RecordMeta { return &p.RecordMeta }),
		bySubject: make(map[string]uuid.UUID),
	}
}

func prefKey(subjectType, subjectID, definitionCode, channel string) string {
	return strings.ToLower(fmt.Sprintf("%s|%s|%s|%s", subjectType, subjectID, definitionCode, channel))
}

func (r *PreferenceRepository) Create(ctx context.Context, pref *domain.NotificationPreference) error {
	key := prefKey(pref.SubjectType, pref.SubjectID, pref.DefinitionCode, pref.Channel)
	if _, ok := r.bySubject[key]; ok {
		return fmt.Errorf("preference already exists for %s", key)
	}
	if err := r.base.create(ctx, pref); err != nil {
		return err
	}
	r.bySubject[key] = pref.ID
	return nil
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
	key := prefKey(subjectType, subjectID, definitionCode, channel)
	id, ok := r.bySubject[key]
	if !ok {
		return nil, store.ErrNotFound
	}
	return r.base.getByID(ctx, id, false)
}
