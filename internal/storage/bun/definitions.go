package bunrepo

import (
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/uptrace/bun"
)

type DefinitionRepository struct {
	codedRepository[domain.NotificationDefinition]
}

func NewDefinitionRepository(db *bun.DB) *DefinitionRepository {
	return &DefinitionRepository{
		codedRepository: newCodedRepository(db,
			func(definition *domain.NotificationDefinition) *domain.RecordMeta { return &definition.RecordMeta },
			func(definition *domain.NotificationDefinition) string { return definition.Code }),
	}
}
