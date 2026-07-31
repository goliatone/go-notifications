package bunrepo

import (
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/uptrace/bun"
)

type SubscriptionRepository struct {
	codedRepository[domain.SubscriptionGroup]
}

func NewSubscriptionRepository(db *bun.DB) *SubscriptionRepository {
	return &SubscriptionRepository{
		codedRepository: newCodedRepository(db,
			func(group *domain.SubscriptionGroup) *domain.RecordMeta { return &group.RecordMeta },
			func(group *domain.SubscriptionGroup) string { return group.Code }),
	}
}
