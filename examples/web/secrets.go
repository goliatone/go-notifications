package main

import (
	"fmt"
	"os"
	"time"

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
			lgr.Warn("invalid DEMO_SECRET_KEY length, using default", "length", len(key))
		}
		key = defaultSecretKey
	}
	secretStore := bunrepo.NewSecretStore(db)
	provider, err := secrets.NewEncryptedStoreProvider(secretStore, []byte(key))
	if err != nil {
		return nil, nil, fmt.Errorf("secrets: provider: %w", err)
	}
	var resolver secrets.Resolver = secrets.SimpleResolver{Provider: provider}
	cacheTTL := parseCacheTTL(os.Getenv("SECRETS_CACHE_TTL"))
	if cacheTTL > 0 {
		resolver = secrets.NewCachingResolver(resolver, cacheTTL)
	}
	return provider, resolver, nil
}

func parseCacheTTL(raw string) time.Duration {
	if raw == "" {
		return 30 * time.Second
	}
	if ttl, err := time.ParseDuration(raw); err == nil && ttl > 0 {
		return ttl
	}
	return 0
}
