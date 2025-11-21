package main

import (
	"fmt"
	"os"

	bunrepo "github.com/goliatone/go-notifications/internal/storage/bun"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/chacha20poly1305"
)

const defaultSecretKey = "0123456789abcdef0123456789abcdef"

func buildSecretsProvider(db *bun.DB, lgr logger.Logger) (secrets.Provider, secrets.Resolver, error) {
	key := os.Getenv("DEMO_SECRET_KEY")
	if len(key) != chacha20poly1305.KeySize {
		if key != "" && lgr != nil {
			lgr.Warn("invalid DEMO_SECRET_KEY length, using default", logger.Field{Key: "length", Value: len(key)})
		}
		key = defaultSecretKey
	}
	secretStore := bunrepo.NewSecretStore(db)
	provider, err := secrets.NewEncryptedStoreProvider(secretStore, []byte(key))
	if err != nil {
		return nil, nil, fmt.Errorf("secrets: provider: %w", err)
	}
	return provider, secrets.SimpleResolver{Provider: provider}, nil
}
