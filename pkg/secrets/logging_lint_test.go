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

	err := filepath.WalkDir(root, secretLogFileVisitor(secretSet))
	if err != nil {
		t.Fatalf("log-safety lint failed: %v", err)
	}
}

func secretLogFileVisitor(secretSet map[string]struct{}) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".tmp", "tmp", "vendor", "node_modules":
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
		lintErr := inspectSecretLogCalls(file, path, secretSet)
		if lintErr != nil {
			return lintErr
		}
		return nil
	}
}

func inspectSecretLogCalls(file *ast.File, path string, secretSet map[string]struct{}) error {
	var lintErr error
	ast.Inspect(file, func(node ast.Node) bool {
		if lintErr != nil {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || !isLoggerCall(call) {
			return true
		}
		if key := loggedSecretKey(call.Args, secretSet); key != "" {
			lintErr = fmt.Errorf("secret-like field %q logged in %s; mask it or drop the field", key, path)
			return false
		}
		return true
	})
	return lintErr
}

func isLoggerCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch selector.Sel.Name {
	case "Trace", "Debug", "Info", "Warn", "Error", "Fatal":
		return true
	default:
		return false
	}
}

func loggedSecretKey(args []ast.Expr, secretSet map[string]struct{}) string {
	for index, argument := range args {
		if index == 0 {
			continue
		}
		literal, ok := argument.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}
		key, err := strconv.Unquote(literal.Value)
		if err == nil {
			if _, exists := secretSet[key]; exists {
				return key
			}
		}
	}
	return ""
}
