package notifications_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOpenPGPDependencyGuard(t *testing.T) {
	taskfile, err := filepath.Abs("taskfile")
	if err != nil {
		t.Fatalf("resolve taskfile: %v", err)
	}
	cases := []struct {
		name       string
		importPath string
		testOnly   bool
		task       string
		wantError  string
	}{
		{name: "safe_crypto", importPath: "golang.org/x/crypto/chacha20poly1305"},
		{name: "similar_prefix", importPath: "golang.org/x/crypto/openpgpextra"},
		{name: "direct", importPath: "golang.org/x/crypto/openpgp", wantError: "GO-2026-5932"},
		{name: "subpackage", importPath: "golang.org/x/crypto/openpgp/packet", wantError: "GO-2026-5932"},
		{name: "transitive", importPath: "golang.org/x/crypto/bridge", wantError: "GO-2026-5932"},
		{name: "test_only", importPath: "golang.org/x/crypto/openpgp", testOnly: true, wantError: "GO-2026-5932"},
		{name: "test_transitive", importPath: "golang.org/x/crypto/bridge", testOnly: true, wantError: "GO-2026-5932"},
		{name: "graph_error", importPath: "fixture.invalid/missing", wantError: "Unable to inspect Go dependencies"},
		{name: "security_pipeline", importPath: "golang.org/x/crypto/openpgp", testOnly: true, task: "go:security:all", wantError: "GO-2026-5932"},
	}
	for _, module := range []string{".", "examples/web"} {
		for _, tc := range cases {
			t.Run(module+"/"+tc.name, func(t *testing.T) {
				root := openPGPGuardFixture(t)
				sourceName, packageName := "imports.go", "fixture"
				if tc.testOnly {
					sourceName, packageName = "imports_test.go", "fixture_test"
				}
				writeGuardFixture(t, filepath.Join(root, module, sourceName),
					fmt.Sprintf("package %s\n\nimport _ %q\n", packageName, tc.importPath))
				task := tc.task
				if task == "" {
					task = "go:security:openpgp"
				}
				cmd := exec.CommandContext(t.Context(), "bash", taskfile, task)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
				output, runErr := cmd.CombinedOutput()
				if tc.wantError == "" {
					if runErr != nil {
						t.Fatalf("guard rejected allowed graph: %v\n%s", runErr, output)
					}
					return
				}
				if runErr == nil || !strings.Contains(string(output), tc.wantError) {
					t.Fatalf("want failure containing %q; got %v\n%s", tc.wantError, runErr, output)
				}
				if !strings.Contains(string(output), filepath.Join(root, module)) {
					t.Fatalf("failure does not identify affected module %q:\n%s", module, output)
				}
			})
		}
	}
}

func openPGPGuardFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	crypto := filepath.Join(dir, "crypto")
	writeGuardFixture(t, filepath.Join(crypto, "go.mod"), "module golang.org/x/crypto\n\ngo 1.26.0\n")
	for path, source := range map[string]string{
		"openpgp":          "package openpgp\n",
		"openpgp/packet":   "package packet\n",
		"chacha20poly1305": "package chacha20poly1305\n",
		"openpgpextra":     "package openpgpextra\n",
		"bridge":           "package bridge\n\nimport _ \"golang.org/x/crypto/openpgp\"\n",
	} {
		writeGuardFixture(t, filepath.Join(crypto, path, "stub.go"), source)
	}
	root := filepath.Join(dir, "repo")
	for _, module := range []string{".", "examples/web"} {
		mod := fmt.Sprintf("module fixture.test/%s\n\ngo 1.26.0\n\nrequire golang.org/x/crypto v0.0.0\n\nreplace golang.org/x/crypto => %s\n",
			filepath.Base(filepath.Join(root, module)), strconv.Quote(crypto))
		writeGuardFixture(t, filepath.Join(root, module, "go.mod"), mod)
		writeGuardFixture(t, filepath.Join(root, module, "fixture.go"), "package fixture\n")
	}
	return root
}

func writeGuardFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
