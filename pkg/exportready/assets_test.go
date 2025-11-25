package exportready

import (
	"context"
	"errors"
	"strings"
	"testing"

	i18n "github.com/goliatone/go-i18n"
	memstore "github.com/goliatone/go-notifications/internal/storage/memory"
	internaltemplates "github.com/goliatone/go-notifications/internal/templates"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	pkgtemplates "github.com/goliatone/go-notifications/pkg/templates"
)

func TestDefinitionAndTemplatesShape(t *testing.T) {
	def := Definition()
	if def.Code != DefinitionCode {
		t.Fatalf("unexpected definition code: %s", def.Code)
	}
	if !contains(def.Channels, "email") || !contains(def.Channels, "in-app") {
		t.Fatalf("expected email and in-app channels, got %v", def.Channels)
	}
	if !contains(def.TemplateKeys, "email:"+EmailTemplateCode) || !contains(def.TemplateKeys, "in-app:"+InAppTemplateCode) {
		t.Fatalf("template keys missing expected entries: %v", def.TemplateKeys)
	}

	tpls := Templates()
	if len(tpls) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(tpls))
	}
	for _, tpl := range tpls {
		if tpl.Schema.Required == nil || len(tpl.Schema.Required) == 0 {
			t.Fatalf("template %s missing required schema", tpl.Code)
		}
		if !equalStringSets(tpl.Schema.Required, templateSchema.Required) {
			t.Fatalf("template %s required schema mismatch", tpl.Code)
		}
		if !equalStringSets(tpl.Schema.Optional, templateSchema.Optional) {
			t.Fatalf("template %s optional schema mismatch", tpl.Code)
		}
	}
	email := baseTemplateFor(tpls, "email")
	if email.Metadata["cta_label"] != "Download" || email.Metadata["icon"] == "" {
		t.Fatalf("expected email metadata defaults, got %v", email.Metadata)
	}
	inapp := baseTemplateFor(tpls, "in-app")
	if inapp.Metadata["cta_label"] != "Open" || inapp.Metadata["badge"] == "" {
		t.Fatalf("expected in-app metadata defaults, got %v", inapp.Metadata)
	}
}

func TestTemplatesRenderEmailAndInApp(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	for _, tpl := range Templates() {
		copy := tpl
		if err := repo.Create(ctx, &copy); err != nil {
			t.Fatalf("seed template: %v", err)
		}
	}

	translator := newTranslator(t)
	svc, err := pkgtemplates.New(pkgtemplates.Dependencies{
		Repository:    repo,
		Cache:         &cache.Nop{},
		Logger:        &logger.Nop{},
		Translator:    translator,
		Fallbacks:     i18n.NewStaticFallbackResolver(),
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("template service: %v", err)
	}

	payload := map[string]any{
		"FileName":    "orders.csv",
		"Format":      "csv",
		"URL":         "https://example.com/exports/orders.csv",
		"ExpiresAt":   "2024-05-01T00:00:00Z",
		"Rows":        1200,
		"Parts":       3,
		"ManifestURL": "https://example.com/exports/manifest.json",
		"Message":     "Filtered to active customers",
	}

	email, err := svc.Render(ctx, pkgtemplates.RenderRequest{
		Code:    EmailTemplateCode,
		Channel: "email",
		Locale:  "en",
		Data:    payload,
	})
	if err != nil {
		t.Fatalf("render email: %v", err)
	}
	if !strings.Contains(email.Subject, `orders.csv`) {
		t.Fatalf("email subject missing filename: %s", email.Subject)
	}
	if !strings.Contains(email.Body, payload["URL"].(string)) {
		t.Fatalf("email body missing URL: %s", email.Body)
	}
	if !strings.Contains(email.Body, payload["ManifestURL"].(string)) {
		t.Fatalf("email body missing manifest URL: %s", email.Body)
	}
	if !strings.Contains(email.Body, "Rows: 1200") || !strings.Contains(email.Body, "Parts: 3") {
		t.Fatalf("email body missing counts: %s", email.Body)
	}

	inapp, err := svc.Render(ctx, pkgtemplates.RenderRequest{
		Code:    InAppTemplateCode,
		Channel: "in-app",
		Locale:  "en",
		Data: map[string]any{
			"FileName":  payload["FileName"],
			"Format":    payload["Format"],
			"URL":       payload["URL"],
			"ExpiresAt": payload["ExpiresAt"],
		},
	})
	if err != nil {
		t.Fatalf("render in-app: %v", err)
	}
	if strings.Contains(inapp.Body, "Rows:") || strings.Contains(inapp.Body, "Parts:") || strings.Contains(inapp.Body, "Manifest") {
		t.Fatalf("in-app body should omit optional fields when absent: %s", inapp.Body)
	}
	if !strings.Contains(inapp.Subject, payload["FileName"].(string)) {
		t.Fatalf("in-app subject missing filename: %s", inapp.Subject)
	}
}

func TestTemplatesRespectChannelOverrides(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	for _, tpl := range Templates() {
		copy := tpl
		if err := repo.Create(ctx, &copy); err != nil {
			t.Fatalf("seed template: %v", err)
		}
	}
	translator := newTranslator(t)
	svc, err := pkgtemplates.New(pkgtemplates.Dependencies{
		Repository:    repo,
		Cache:         &cache.Nop{},
		Logger:        &logger.Nop{},
		Translator:    translator,
		Fallbacks:     i18n.NewStaticFallbackResolver(),
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("template service: %v", err)
	}

	payload := map[string]any{
		"FileName":  "export.json",
		"Format":    "json",
		"URL":       "https://example.com/export.json",
		"ExpiresAt": "2024-06-01T00:00:00Z",
		"channel_overrides": map[string]map[string]any{
			"email": {
				"cta_label":  "Download now",
				"action_url": "https://cdn.example.com/export.json",
			},
			"in-app": {
				"cta_label":  "Open link",
				"action_url": "https://cdn.example.com/export.json",
			},
		},
	}

	preparePayloadForChannel(payload, "email")
	email, err := svc.Render(ctx, pkgtemplates.RenderRequest{
		Code:    EmailTemplateCode,
		Channel: "email",
		Locale:  "en",
		Data:    payload,
	})
	if err != nil {
		t.Fatalf("render email: %v", err)
	}
	if !strings.Contains(email.Body, "Download now") || !strings.Contains(email.Body, "cdn.example.com") {
		t.Fatalf("email did not apply overrides: %s", email.Body)
	}

	preparePayloadForChannel(payload, "in-app")
	inapp, err := svc.Render(ctx, pkgtemplates.RenderRequest{
		Code:    InAppTemplateCode,
		Channel: "in-app",
		Locale:  "en",
		Data:    payload,
	})
	if err != nil {
		t.Fatalf("render in-app: %v", err)
	}
	if !strings.Contains(inapp.Body, "Open link") || !strings.Contains(inapp.Body, "cdn.example.com") {
		t.Fatalf("in-app did not apply overrides: %s", inapp.Body)
	}
}

func TestSchemaEnforcedForRequiredFields(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	for _, tpl := range Templates() {
		copy := tpl
		if err := repo.Create(ctx, &copy); err != nil {
			t.Fatalf("seed template: %v", err)
		}
	}
	translator := newTranslator(t)
	svc, err := pkgtemplates.New(pkgtemplates.Dependencies{
		Repository:    repo,
		Cache:         &cache.Nop{},
		Logger:        &logger.Nop{},
		Translator:    translator,
		Fallbacks:     i18n.NewStaticFallbackResolver(),
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("template service: %v", err)
	}

	_, err = svc.Render(ctx, pkgtemplates.RenderRequest{
		Code:    EmailTemplateCode,
		Channel: "email",
		Locale:  "en",
		Data: map[string]any{
			"FileName": "orders.csv",
			"Format":   "csv",
			"URL":      "https://example.com/exports/orders.csv",
			// ExpiresAt missing
		},
	})
	var schemaErr internaltemplates.SchemaError
	if err == nil || !errors.As(err, &schemaErr) {
		t.Fatalf("expected schema error for missing fields, got %v", err)
	}
	if len(schemaErr.Missing) == 0 {
		t.Fatalf("expected missing fields to be reported")
	}
}

func contains(list domain.StringList, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, val := range a {
		seen[val] = struct{}{}
	}
	for _, val := range b {
		if _, ok := seen[val]; !ok {
			return false
		}
	}
	return true
}
