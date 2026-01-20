package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	memstore "github.com/goliatone/go-notifications/internal/storage/memory"
	internaltemplates "github.com/goliatone/go-notifications/internal/templates"
	"github.com/goliatone/go-notifications/pkg/domain"
	pkgtemplates "github.com/goliatone/go-notifications/pkg/templates"
)

func TestTemplatesShape(t *testing.T) {
	tpls := Templates()
	if len(tpls) != 4 {
		t.Fatalf("expected 4 templates, got %d", len(tpls))
	}

	expected := map[string]domain.TemplateSchema{
		PasswordResetCode:     passwordResetSchema,
		InviteCode:            inviteSchema,
		AccountLockoutCode:    accountLockoutSchema,
		EmailVerificationCode: emailVerificationSchema,
	}

	for _, tpl := range tpls {
		schema, ok := expected[tpl.Code]
		if !ok {
			t.Fatalf("unexpected template code: %s", tpl.Code)
		}
		if tpl.Channel != "email" {
			t.Fatalf("expected email channel for %s, got %s", tpl.Code, tpl.Channel)
		}
		if !equalStringSets(tpl.Schema.Required, schema.Required) {
			t.Fatalf("template %s required schema mismatch", tpl.Code)
		}
		if !equalStringSets(tpl.Schema.Optional, schema.Optional) {
			t.Fatalf("template %s optional schema mismatch", tpl.Code)
		}
	}
}

func TestTemplatesRender(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	for _, tpl := range Templates() {
		copy := tpl
		if err := repo.Create(ctx, &copy); err != nil {
			t.Fatalf("seed template: %v", err)
		}
	}
	svc := newTemplateService(t, repo)

	cases := []struct {
		name      string
		code      string
		data      map[string]any
		want      []string
		wantLower []string
		wantValue any
		notWant   []string
	}{
		{
			name: "password reset with minutes",
			code: PasswordResetCode,
			data: map[string]any{
				"name":              "Alex",
				"app_name":          "Acme",
				"action_url":        "https://example.com/reset",
				"expires_at":        "2024-05-01T00:00:00Z",
				"remaining_minutes": 30,
			},
			want:      []string{"https://example.com/reset"},
			wantValue: 30,
		},
		{
			name: "invite without minutes",
			code: InviteCode,
			data: map[string]any{
				"name":       "Sam",
				"app_name":   "Acme",
				"action_url": "https://example.com/invite",
				"expires_at": "2024-06-01T00:00:00Z",
			},
			want:    []string{"https://example.com/invite", "2024-06-01T00:00:00Z"},
			notWant: []string{"about"},
		},
		{
			name: "account lockout",
			code: AccountLockoutCode,
			data: map[string]any{
				"name":          "Jordan",
				"app_name":      "Acme",
				"reason":        "Too many failed attempts",
				"lockout_until": "2024-07-01T00:00:00Z",
				"unlock_url":    "https://example.com/unlock",
			},
			want: []string{"Too many failed attempts", "https://example.com/unlock"},
		},
		{
			name: "email verification",
			code: EmailVerificationCode,
			data: map[string]any{
				"name":           "Toni",
				"app_name":       "Acme",
				"verify_url":     "https://example.com/verify",
				"expires_at":     "2024-08-01T00:00:00Z",
				"resend_allowed": true,
			},
			want:      []string{"https://example.com/verify"},
			wantLower: []string{"resend allowed: true"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered, err := svc.Render(ctx, pkgtemplates.RenderRequest{
				Code:    tc.code,
				Channel: "email",
				Locale:  "en",
				Data:    tc.data,
			})
			if err != nil {
				t.Fatalf("render %s: %v", tc.code, err)
			}
			for _, want := range tc.want {
				if !strings.Contains(rendered.Body, want) && !strings.Contains(rendered.Subject, want) {
					t.Fatalf("rendered output missing %q", want)
				}
			}
			if tc.wantValue != nil {
				value := fmt.Sprintf("%v", tc.wantValue)
				if !strings.Contains(rendered.Body, value) && !strings.Contains(rendered.Subject, value) {
					t.Fatalf("rendered output missing value %q", value)
				}
			}
			for _, want := range tc.wantLower {
				if !strings.Contains(strings.ToLower(rendered.Body), want) {
					t.Fatalf("rendered output missing %q", want)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(rendered.Body, notWant) {
					t.Fatalf("rendered output should not include %q", notWant)
				}
			}
		})
	}
}

func TestSchemaEnforcesRequiredFields(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	for _, tpl := range Templates() {
		copy := tpl
		if err := repo.Create(ctx, &copy); err != nil {
			t.Fatalf("seed template: %v", err)
		}
	}
	svc := newTemplateService(t, repo)

	_, err := svc.Render(ctx, pkgtemplates.RenderRequest{
		Code:    PasswordResetCode,
		Channel: "email",
		Locale:  "en",
		Data: map[string]any{
			"name":       "Alex",
			"app_name":   "Acme",
			"expires_at": "2024-05-01T00:00:00Z",
			// action_url missing
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
