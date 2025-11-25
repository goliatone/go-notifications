package onready

import (
	"context"
	"errors"
	"testing"

	memstore "github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
)

func TestRegisterIdempotent(t *testing.T) {
	ctx := context.Background()
	defRepo := memstore.NewDefinitionRepository()
	tplRepo := memstore.NewTemplateRepository()
	tplSvc := newTemplateService(t, tplRepo)

	result, err := Register(ctx, Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
	}, Options{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.DefinitionCode != DefinitionCode {
		t.Fatalf("unexpected definition code: %s", result.DefinitionCode)
	}

	// Ensure templates exist
	emailTpl, err := tplSvc.Get(ctx, EmailTemplateCode, "email", "en")
	if err != nil {
		t.Fatalf("get email template: %v", err)
	}
	inappTpl, err := tplSvc.Get(ctx, InAppTemplateCode, "in-app", "en")
	if err != nil {
		t.Fatalf("get in-app template: %v", err)
	}
	if emailTpl.Revision != 1 || inappTpl.Revision != 1 {
		t.Fatalf("expected initial revision 1, got email=%d inapp=%d", emailTpl.Revision, inappTpl.Revision)
	}
	if label := emailTpl.Metadata["cta_label"]; label != "Download" {
		t.Fatalf("expected default cta_label, got %v", label)
	}

	// Re-run to ensure idempotency (no new revisions/records)
	if _, err := Register(ctx, Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
	}, Options{}); err != nil {
		t.Fatalf("second register: %v", err)
	}

	defs, err := defRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list definitions: %v", err)
	}
	if defs.Total != 1 {
		t.Fatalf("expected 1 definition, got %d", defs.Total)
	}

	templates, err := tplRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if templates.Total != 2 {
		t.Fatalf("expected 2 templates, got %d", templates.Total)
	}

	emailTplAfter, _ := tplSvc.Get(ctx, EmailTemplateCode, "email", "en")
	if emailTplAfter.Revision != emailTpl.Revision {
		t.Fatalf("expected email template revision unchanged, got %d", emailTplAfter.Revision)
	}
}

func TestRegisterSupportsNamespaceAndOverrides(t *testing.T) {
	ctx := context.Background()
	defRepo := memstore.NewDefinitionRepository()
	tplRepo := memstore.NewTemplateRepository()
	tplSvc := newTemplateService(t, tplRepo)

	opts := Options{
		Namespace:             "billing",
		DefinitionName:        "Billing Export Ready",
		DefinitionDescription: "Billing export notification",
		Channels:              []string{"email"}, // omit in-app
		EmailSubject:          "Billing export ready",
		EmailBody:             "Custom body",
		EmailCTALabel:         "Download now",
		EmailIcon:             "download",
		InAppCTALabel:         "Open export",
	}

	_, err := Register(ctx, Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
	}, opts)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	def, err := defRepo.GetByCode(ctx, "billing."+DefinitionCode)
	if err != nil {
		t.Fatalf("get definition: %v", err)
	}
	if def.Name != opts.DefinitionName {
		t.Fatalf("expected definition name override, got %s", def.Name)
	}
	if len(def.Channels) != 1 || def.Channels[0] != "email" {
		t.Fatalf("expected only email channel, got %v", def.Channels)
	}
	if len(def.TemplateKeys) != 1 || def.TemplateKeys[0] != "email:billing."+EmailTemplateCode {
		t.Fatalf("unexpected template keys: %v", def.TemplateKeys)
	}

	emailTpl, err := tplSvc.Get(ctx, "billing."+EmailTemplateCode, "email", "en")
	if err != nil {
		t.Fatalf("get email template: %v", err)
	}
	if emailTpl.Subject != opts.EmailSubject {
		t.Fatalf("expected email subject override, got %s", emailTpl.Subject)
	}
	if emailTpl.Body != opts.EmailBody {
		t.Fatalf("expected email body override, got %s", emailTpl.Body)
	}
	if emailTpl.Metadata["cta_label"] != opts.EmailCTALabel {
		t.Fatalf("expected email CTA label override, got %v", emailTpl.Metadata["cta_label"])
	}
	if emailTpl.Metadata["icon"] != opts.EmailIcon {
		t.Fatalf("expected email icon override, got %v", emailTpl.Metadata["icon"])
	}

	if _, err := tplSvc.Get(ctx, "billing."+InAppTemplateCode, "in-app", "en"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected no in-app template when channel omitted, got %v", err)
	}
}
