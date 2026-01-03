package secrets

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
	secretSet := make(map[string]struct{}, len(secretKeys))
	for _, key := range secretKeys {
		secretSet[key] = struct{}{}
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
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, data, 0)
		if err != nil {
			return err
		}
		var lintErr error
		ast.Inspect(file, func(n ast.Node) bool {
			if lintErr != nil {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "Trace", "Debug", "Info", "Warn", "Error", "Fatal":
			default:
				return true
			}
			for i, arg := range call.Args {
				if i == 0 {
					continue
				}
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				key, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				if _, exists := secretSet[key]; exists {
					lintErr = fmt.Errorf("secret-like field %q logged in %s; mask it or drop the field", key, path)
					return false
				}
			}
			return true
		})
		if lintErr != nil {
			return lintErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("log-safety lint failed: %v", err)
	}
}
