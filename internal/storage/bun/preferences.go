package bunrepo

import (
	"context"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/uptrace/bun"
)

type PreferenceRepository struct {
	crudRepository[domain.NotificationPreference]
}

func NewPreferenceRepository(db *bun.DB) *PreferenceRepository {
	return &PreferenceRepository{
		crudRepository: newEntityCRUD(db, "id",
			func(preference *domain.NotificationPreference) *domain.RecordMeta { return &preference.RecordMeta }, nil),
	}
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
