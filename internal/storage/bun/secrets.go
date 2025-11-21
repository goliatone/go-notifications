package bunrepo

import (
	"context"
	"time"

	iface "github.com/goliatone/go-notifications/pkg/interfaces/secrets"
	"github.com/uptrace/bun"
)

type secretRecord struct {
	bun.BaseModel `bun:"table:secrets"`

	ID        int64          `bun:",pk,autoincrement"`
	Scope     string         `bun:",notnull,unique:secret_identity"`
	SubjectID string         `bun:",notnull,unique:secret_identity"`
	Channel   string         `bun:",notnull,unique:secret_identity"`
	Provider  string         `bun:",notnull,unique:secret_identity"`
	Key       string         `bun:",notnull,unique:secret_identity"`
	Version   string         `bun:",notnull,unique:secret_identity"`
	Cipher    []byte         `bun:",notnull"`
	Nonce     []byte         `bun:",notnull"`
	Metadata  map[string]any `bun:",type:jsonb"`
	CreatedAt time.Time      `bun:",nullzero,notnull,default:current_timestamp"`
	UpdatedAt time.Time      `bun:",nullzero,notnull,default:current_timestamp"`
	DeletedAt bun.NullTime   `bun:",soft_delete,nullzero"`
}

type SecretStore struct {
	db *bun.DB
}

func NewSecretStore(db *bun.DB) *SecretStore {
	return &SecretStore{db: db}
}

func (s *SecretStore) Put(ctx context.Context, rec iface.Record) error {
	model := toSecretRecord(rec)
	_, err := s.db.NewInsert().
		Model(model).
		On("CONFLICT (scope, subject_id, channel, provider, key, version) DO UPDATE").
		Set("cipher = EXCLUDED.cipher").
		Set("nonce = EXCLUDED.nonce").
		Set("metadata = EXCLUDED.metadata").
		Set("updated_at = current_timestamp").
		Exec(ctx)
	return err
}

func (s *SecretStore) GetLatest(ctx context.Context, scope, subjectID, channel, provider, key string) (iface.Record, error) {
	var rec secretRecord
	err := s.db.NewSelect().
		Model(&rec).
		Where("scope = ? AND subject_id = ? AND channel = ? AND provider = ? AND key = ?", scope, subjectID, channel, provider, key).
		Where("deleted_at IS NULL").
		OrderExpr("version DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return iface.Record{}, err
	}
	return fromSecretRecord(rec), nil
}

func (s *SecretStore) GetVersion(ctx context.Context, scope, subjectID, channel, provider, key, version string) (iface.Record, error) {
	var rec secretRecord
	err := s.db.NewSelect().
		Model(&rec).
		Where("scope = ? AND subject_id = ? AND channel = ? AND provider = ? AND key = ? AND version = ?", scope, subjectID, channel, provider, key, version).
		Where("deleted_at IS NULL").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return iface.Record{}, err
	}
	return fromSecretRecord(rec), nil
}

func (s *SecretStore) Delete(ctx context.Context, scope, subjectID, channel, provider, key string) error {
	_, err := s.db.NewDelete().
		Model((*secretRecord)(nil)).
		Where("scope = ? AND subject_id = ? AND channel = ? AND provider = ? AND key = ?", scope, subjectID, channel, provider, key).
		Exec(ctx)
	return err
}

func (s *SecretStore) List(ctx context.Context, scope, subjectID, channel, provider, key string) ([]iface.Record, error) {
	var recs []secretRecord
	query := s.db.NewSelect().Model(&recs).Where("deleted_at IS NULL")
	if scope != "" {
		query = query.Where("scope = ?", scope)
	}
	if subjectID != "" {
		query = query.Where("subject_id = ?", subjectID)
	}
	if channel != "" {
		query = query.Where("channel = ?", channel)
	}
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if key != "" {
		query = query.Where("key = ?", key)
	}
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	results := make([]iface.Record, 0, len(recs))
	for _, r := range recs {
		results = append(results, fromSecretRecord(r))
	}
	return results, nil
}

func toSecretRecord(rec iface.Record) *secretRecord {
	return &secretRecord{
		Scope:     rec.Scope,
		SubjectID: rec.SubjectID,
		Channel:   rec.Channel,
		Provider:  rec.Provider,
		Key:       rec.Key,
		Version:   rec.Version,
		Cipher:    rec.Cipher,
		Nonce:     rec.Nonce,
		Metadata:  rec.Metadata,
	}
}

func fromSecretRecord(rec secretRecord) iface.Record {
	return iface.Record{
		Scope:     rec.Scope,
		SubjectID: rec.SubjectID,
		Channel:   rec.Channel,
		Provider:  rec.Provider,
		Key:       rec.Key,
		Version:   rec.Version,
		Cipher:    rec.Cipher,
		Nonce:     rec.Nonce,
		Metadata:  rec.Metadata,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
		DeletedAt: rec.DeletedAt,
	}
}
