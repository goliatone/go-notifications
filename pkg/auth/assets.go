package auth

import "github.com/goliatone/go-notifications/pkg/domain"

const (
	PasswordResetCode     = "password-reset"
	InviteCode            = "invite"
	AccountLockoutCode    = "account-lockout"
	EmailVerificationCode = "email-verification"
)

var (
	passwordResetSchema = domain.TemplateSchema{
		Required: []string{"name", "action_url", "expires_at", "app_name"},
		Optional: []string{"remaining_minutes"},
	}
	inviteSchema = domain.TemplateSchema{
		Required: []string{"name", "app_name", "action_url", "expires_at"},
		Optional: []string{"remaining_minutes"},
	}
	accountLockoutSchema = domain.TemplateSchema{
		Required: []string{"name", "reason", "lockout_until", "unlock_url", "app_name"},
	}
	emailVerificationSchema = domain.TemplateSchema{
		Required: []string{"name", "verify_url", "expires_at", "resend_allowed", "app_name"},
	}
)

// Templates returns default auth/onboarding email templates.
func Templates() []domain.NotificationTemplate {
	return []domain.NotificationTemplate{
		{
			Code:        PasswordResetCode,
			Channel:     "email",
			Locale:      "en",
			Subject:     "Reset your {{ app_name }} password",
			Body:        passwordResetBody,
			Description: "Email template for password reset notifications",
			Format:      "text/plain",
			Schema:      passwordResetSchema,
			Metadata: domain.JSONMap{
				"category": "auth",
				"cta_label": "Reset password",
			},
		},
		{
			Code:        InviteCode,
			Channel:     "email",
			Locale:      "en",
			Subject:     "You're invited to {{ app_name }}",
			Body:        inviteBody,
			Description: "Email template for user invite notifications",
			Format:      "text/plain",
			Schema:      inviteSchema,
			Metadata: domain.JSONMap{
				"category": "onboarding",
				"cta_label": "Accept invite",
			},
		},
		{
			Code:        AccountLockoutCode,
			Channel:     "email",
			Locale:      "en",
			Subject:     "Account locked for {{ app_name }}",
			Body:        accountLockoutBody,
			Description: "Email template for account lockout notifications",
			Format:      "text/plain",
			Schema:      accountLockoutSchema,
			Metadata: domain.JSONMap{
				"category": "auth",
				"cta_label": "Unlock account",
			},
		},
		{
			Code:        EmailVerificationCode,
			Channel:     "email",
			Locale:      "en",
			Subject:     "Verify your {{ app_name }} email",
			Body:        emailVerificationBody,
			Description: "Email template for email verification notifications",
			Format:      "text/plain",
			Schema:      emailVerificationSchema,
			Metadata: domain.JSONMap{
				"category": "auth",
				"cta_label": "Verify email",
			},
		},
	}
}

const passwordResetBody = `
Hi {{ name }},

We received a request to reset your password.

Click the link below to set a new password{% if remaining_minutes %} (expires at {{ expires_at }}, about {{ remaining_minutes }} minutes from now){% else %} (expires at {{ expires_at }}){% endif %}:
{{ action_url }}

If you didn't request this, you can ignore this email.

- The {{ app_name }} Team
`

const inviteBody = `
Hi {{ name }},

You've been invited to join {{ app_name }}.

Accept your invite here{% if remaining_minutes %} (expires at {{ expires_at }}, about {{ remaining_minutes }} minutes from now){% else %} (expires at {{ expires_at }}){% endif %}:
{{ action_url }}

- The {{ app_name }} Team
`

const accountLockoutBody = `
Hi {{ name }},

Your account was locked due to {{ reason }} until {{ lockout_until }}.

Unlock your account here:
{{ unlock_url }}

- The {{ app_name }} Team
`

const emailVerificationBody = `
Hi {{ name }},

Please verify your email address:
{{ verify_url }}

This link expires at {{ expires_at }}.
Resend allowed: {{ resend_allowed }}

- The {{ app_name }} Team
`
