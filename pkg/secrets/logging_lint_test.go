package secrets

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNoSecretFieldsLoggedDirectly(t *testing.T) {
	secretKeys := []string{
		"token", "access_token", "refresh_token",
		"api_key", "apikey", "apiKey",
		"client_secret", "signing_key",
		"chat_id", "webhook_url",
	}

	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "tmp", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "logging_lint_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		for _, key := range secretKeys {
			needle := fmt.Sprintf(`logger.Field{Key: "%s"`, key)
			if strings.Contains(content, needle) {
				return fmt.Errorf("secret-like field %q logged in %s; mask it or drop the field", key, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("log-safety lint failed: %v", err)
	}
}
