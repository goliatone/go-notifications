package secrets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	iface "github.com/goliatone/go-notifications/pkg/interfaces/secrets"
	"golang.org/x/crypto/chacha20poly1305"
)

// EncryptedStoreProvider persists secrets encrypted via a Store.
type EncryptedStoreProvider struct {
	store iface.Store
	aead  cipherSuite
	now   func() time.Time
}

type cipherSuite interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
	NonceSize() int
}

// NewEncryptedStoreProvider builds a provider using the given store and key.
func NewEncryptedStoreProvider(store iface.Store, key []byte) (*EncryptedStoreProvider, error) {
	if store == nil {
		return nil, fmt.Errorf("encrypted provider: store required")
	}
	if len(key) != chacha20poly1305.KeySize {
		return nil, fmt.Errorf("encrypted provider: key must be %d bytes", chacha20poly1305.KeySize)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	return &EncryptedStoreProvider{
		store: store,
		aead:  aead,
		now:   time.Now().UTC,
	}, nil
}

func (p *EncryptedStoreProvider) Get(ref Reference) (SecretValue, error) {
	if err := ValidateReference(ref); err != nil {
		return SecretValue{}, err
	}
	ctx := context.Background()
	var rec iface.Record
	var err error
	if ref.Version != "" {
		rec, err = p.store.GetVersion(ctx, string(ref.Scope), ref.SubjectID, ref.Channel, ref.Provider, ref.Key, ref.Version)
	} else {
		rec, err = p.store.GetLatest(ctx, string(ref.Scope), ref.SubjectID, ref.Channel, ref.Provider, ref.Key)
	}
	if err != nil {
		return SecretValue{}, translateStoreError(err)
	}
	plain, err := p.aead.Open(nil, rec.Nonce, rec.Cipher, nil)
	if err != nil {
		return SecretValue{}, fmt.Errorf("decrypt: %w", err)
	}
	return SecretValue{
		Data:      plain,
		Version:   rec.Version,
		Retrieved: p.now(),
		Metadata:  rec.Metadata,
	}, nil
}

func (p *EncryptedStoreProvider) Put(ref Reference, value []byte) (string, error) {
	if err := ValidateReference(ref); err != nil {
		return "", err
	}
	if len(value) == 0 {
		return "", ErrEmptyValue
	}
	if ref.Version == "" {
		ref.Version = p.now().Format(time.RFC3339Nano)
	}
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	cipher := p.aead.Seal(nil, nonce, value, nil)
	rec := iface.Record{
		Scope:     string(ref.Scope),
		SubjectID: ref.SubjectID,
		Channel:   ref.Channel,
		Provider:  ref.Provider,
		Key:       ref.Key,
		Version:   ref.Version,
		Cipher:    cipher,
		Nonce:     nonce,
		Metadata:  map[string]any{"created_at": p.now()},
	}
	if err := p.store.Put(context.Background(), rec); err != nil {
		return "", translateStoreError(err)
	}
	return ref.Version, nil
}

func (p *EncryptedStoreProvider) Delete(ref Reference) error {
	if err := ValidateReference(ref); err != nil {
		return err
	}
	return translateStoreError(p.store.Delete(context.Background(), string(ref.Scope), ref.SubjectID, ref.Channel, ref.Provider, ref.Key))
}

func (p *EncryptedStoreProvider) Describe(ref Reference) (map[string]any, error) {
	if err := ValidateReference(ref); err != nil {
		return nil, err
	}
	rec, err := p.store.GetLatest(context.Background(), string(ref.Scope), ref.SubjectID, ref.Channel, ref.Provider, ref.Key)
	if err != nil {
		return nil, translateStoreError(err)
	}
	return map[string]any{
		"version": rec.Version,
		"meta":    rec.Metadata,
	}, nil
}

func translateStoreError(err error) error {
	switch err {
	case nil:
		return nil
	case sql.ErrNoRows:
		return ErrNotFound
	default:
		return err
	}
}
