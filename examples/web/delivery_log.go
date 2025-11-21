package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/uptrace/bun"
)

// DeliveryLogStore persists recent deliveries so the UI can render provider usage.
type DeliveryLogStore struct {
	db     *bun.DB
	logger logger.Logger
}

// NewDeliveryLogStore wires a Bun-backed recorder.
func NewDeliveryLogStore(db *bun.DB, lgr logger.Logger) *DeliveryLogStore {
	if lgr == nil {
		lgr = &logger.Nop{}
	}
	return &DeliveryLogStore{db: db, logger: lgr}
}

// Record appends a delivery log entry.
func (s *DeliveryLogStore) Record(ctx context.Context, entry deliveryLogRecord) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("delivery log: db not configured")
	}
	_, err := s.db.NewInsert().Model(&entry).Exec(ctx)
	return err
}

// LastForUser returns the latest entries for a user ordered newest first.
func (s *DeliveryLogStore) LastForUser(ctx context.Context, userID string, limit int) ([]deliveryLogRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("delivery log: db not configured")
	}
	if limit <= 0 {
		limit = 5
	}
	var records []deliveryLogRecord
	err := s.db.NewSelect().
		Model(&records).
		Where("user_id = ?", userID).
		OrderExpr("created_at DESC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return []deliveryLogRecord{}, nil
		}
		return nil, err
	}
	return records, nil
}
