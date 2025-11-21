package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/uptrace/bun"
)

var (
	errContactNotFound = errors.New("directory: contact not found")
)

// Directory persists demo contacts and secrets for recipients.
type Directory struct {
	db       *bun.DB
	logger   logger.Logger
	provider secrets.Provider
	resolver secrets.Resolver
}

// NewDirectory builds a directory bound to the SQLite database.
func NewDirectory(db *bun.DB, provider secrets.Provider, resolver secrets.Resolver, lgr logger.Logger) *Directory {
	if lgr == nil {
		lgr = &logger.Nop{}
	}
	return &Directory{db: db, provider: provider, resolver: resolver, logger: lgr}
}

// UpsertContact creates or updates a contact row.
func (d *Directory) UpsertContact(ctx context.Context, contact demoContact) error {
	if d == nil || d.db == nil {
		return errors.New("directory: db not configured")
	}
	contact.UserID = strings.TrimSpace(contact.UserID)
	if contact.UserID == "" {
		return errors.New("directory: user id required")
	}
	_, err := d.db.NewInsert().
		Model(&contact).
		On("CONFLICT (user_id) DO UPDATE").
		Set("display_name = EXCLUDED.display_name").
		Set("email = EXCLUDED.email").
		Set("phone = EXCLUDED.phone").
		Set("slack_id = EXCLUDED.slack_id").
		Set("telegram_chat_id = EXCLUDED.telegram_chat_id").
		Set("preferred_locale = EXCLUDED.preferred_locale").
		Set("tenant_id = EXCLUDED.tenant_id").
		Set("updated_at = CURRENT_TIMESTAMP").
		Exec(ctx)
	return err
}

// GetContact retrieves a contact by user id.
func (d *Directory) GetContact(ctx context.Context, userID string) (*demoContact, error) {
	if d == nil || d.db == nil {
		return nil, errors.New("directory: db not configured")
	}
	var contact demoContact
	err := d.db.NewSelect().
		Model(&contact).
		Where("user_id = ?", strings.TrimSpace(userID)).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errContactNotFound
		}
		return nil, err
	}
	return &contact, nil
}

// PutSecret writes an encrypted secret to the provider.
func (d *Directory) PutSecret(ref secrets.Reference, value []byte) (string, error) {
	if d == nil || d.provider == nil {
		return "", errors.New("directory: secret provider not configured")
	}
	return d.provider.Put(ref, value)
}

// ResolveSecrets fetches scoped secrets via the resolver.
func (d *Directory) ResolveSecrets(refs ...secrets.Reference) (map[secrets.Reference]secrets.SecretValue, error) {
	if d == nil || d.resolver == nil {
		return map[secrets.Reference]secrets.SecretValue{}, secrets.ErrUnsupported
	}
	return d.resolver.Resolve(refs...)
}
