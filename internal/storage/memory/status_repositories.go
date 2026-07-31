package memory

import (
	"github.com/goliatone/go-notifications/pkg/domain"
)

func newDeliveryCRUDRepository() memoryCRUDRepository[domain.DeliveryAttempt] {
	return newMemoryCRUDRepository(
		"delivery_attempt",
		func(attempt *domain.DeliveryAttempt) *domain.RecordMeta { return &attempt.RecordMeta },
		func(attempt *domain.DeliveryAttempt) {
			if attempt.Status == "" {
				attempt.Status = domain.AttemptStatusPending
			}
		},
	)
}

func newMessageCRUDRepository() memoryCRUDRepository[domain.NotificationMessage] {
	return newMemoryCRUDRepository(
		"message",
		func(message *domain.NotificationMessage) *domain.RecordMeta { return &message.RecordMeta },
		func(message *domain.NotificationMessage) {
			if message.Status == "" {
				message.Status = domain.MessageStatusPending
			}
		},
	)
}
