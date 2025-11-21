package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/internal/commands"
	internalprefs "github.com/goliatone/go-notifications/internal/preferences"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/goliatone/go-notifications/pkg/templates"
)

// SeedData seeds the database with demo data.
func SeedData(ctx context.Context, app *App) error {
	if err := seedDefinitions(ctx, app); err != nil {
		return err
	}
	if err := seedTemplates(ctx, app); err != nil {
		return err
	}
	if err := seedContacts(ctx, app); err != nil {
		return err
	}
	if err := seedSecrets(ctx, app); err != nil {
		return err
	}
	if err := seedPreferences(ctx, app); err != nil {
		return err
	}
	if err := seedInboxItems(ctx, app); err != nil {
		return err
	}
	return nil
}

// EnsureSeeded verifies required demo definitions/templates exist, reseeding if needed.
func EnsureSeeded(ctx context.Context, app *App) error {
	if app == nil || app.Module == nil || app.Module.Container() == nil {
		return errors.New("seed: app not initialized")
	}
	container := app.Module.Container()

	defRepo := container.Storage.Definitions
	if defRepo == nil {
		return errors.New("seed: definition repository not available")
	}

	tplRepo := container.Storage.Templates
	if tplRepo == nil {
		return errors.New("seed: template repository not available")
	}

	if _, err := defRepo.GetByCode(ctx, "test_notification"); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		return SeedData(ctx, app)
	}

	if _, err := tplRepo.GetByCodeAndLocale(ctx, "test.in-app", "en", "in-app"); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		return SeedData(ctx, app)
	}
	if _, err := tplRepo.GetByCodeAndLocale(ctx, "test.email", "en", "email"); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		return SeedData(ctx, app)
	}

	return nil
}

// ensureSeededOrError forces seeding and surfaces any errors to clients.
func ensureSeededOrError(ctx context.Context, app *App) error {
	return SeedData(ctx, app)
}

func seedDefinitions(ctx context.Context, app *App) error {
	// Get available channels from adapter registry
	availableChannels := app.AdapterRegistry.GetAvailableChannels()

	// Helper to filter channels based on what's available
	getChannels := func(desired ...string) []string {
		result := make([]string, 0)
		for _, ch := range desired {
			if contains(availableChannels, ch) {
				result = append(result, ch)
			}
		}
		return uniqueStrings(result)
	}

	// Helper to build template IDs based on available channels
	getTemplateIDs := func(code string, channels []string) []string {
		ids := make([]string, 0, len(channels))
		for _, ch := range channels {
			ids = append(ids, ch+":"+code+"."+ch)
		}
		return ids
	}

	definitions := []commands.CreateDefinition{
		{
			Code:        "welcome",
			Name:        "Welcome Message",
			Description: "Welcome new users",
			Severity:    "info",
			Category:    "onboarding",
			Channels:    getChannels("email", "in-app", "slack"),
			TemplateIDs: []string{}, // Will be set below
			AllowUpdate: true,
		},
		{
			Code:        "comment_reply",
			Name:        "Comment Reply",
			Description: "Notify when someone replies to a comment",
			Severity:    "info",
			Category:    "social",
			Channels:    getChannels("in-app", "chat"),
			TemplateIDs: []string{}, // Will be set below
			AllowUpdate: true,
		},
		{
			Code:        "system_alert",
			Name:        "System Alert",
			Description: "Critical system notifications",
			Severity:    "critical",
			Category:    "system",
			Channels:    getChannels("email", "in-app", "sms", "slack"),
			TemplateIDs: []string{}, // Will be set below
			AllowUpdate: true,
		},
		{
			Code:        "test_notification",
			Name:        "Test Notification",
			Description: "For testing multi-channel delivery",
			Severity:    "info",
			Category:    "test",
			Channels:    getChannels("email", "in-app", "chat", "sms"),
			TemplateIDs: []string{}, // Will be set below
			AllowUpdate: true,
		},
		{
			Code:        "admin_message",
			Name:        "Admin Message",
			Description: "Direct message from administrator",
			Severity:    "info",
			Category:    "admin",
			Channels:    getChannels("in-app", "chat"),
			TemplateIDs: []string{}, // Will be set below
			AllowUpdate: true,
		},
	}

	// Set template IDs based on actual channels
	for i := range definitions {
		definitions[i].TemplateIDs = getTemplateIDs(definitions[i].Code, definitions[i].Channels)
	}

	for _, def := range definitions {
		if err := app.Catalog.CreateDefinition.Execute(ctx, def); err != nil {
			// app.Logger.Error("failed to seed definition", "code", def.Code, "error", err)
		}
	}

	return nil
}

func seedTemplates(ctx context.Context, app *App) error {
	templateData := []commands.TemplateUpsert{
		// Welcome email - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "welcome.email",
				Channel: "email",
				Locale:  "en",
				Subject: "Welcome to Notification Center!",
				Body:    "Hello {{ name }},\n\nWelcome to our notification system. You can manage your preferences at any time.\n\nBest regards,\nThe Team",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Welcome email - Spanish
		{
			TemplateInput: templates.TemplateInput{
				Code:    "welcome.email",
				Channel: "email",
				Locale:  "es",
				Subject: "¡Bienvenido al Centro de Notificaciones!",
				Body:    "Hola {{ name }},\n\n¡Bienvenido a nuestro sistema de notificaciones! Puedes gestionar tus preferencias en cualquier momento.\n\nSaludos,\nEl Equipo",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Welcome in-app - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "welcome.in-app",
				Channel: "in-app",
				Locale:  "en",
				Subject: "Welcome!",
				Body:    "Welcome {{ name }}! Thanks for joining our notification center.",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Welcome in-app - Spanish
		{
			TemplateInput: templates.TemplateInput{
				Code:    "welcome.in-app",
				Channel: "in-app",
				Locale:  "es",
				Subject: "¡Bienvenido!",
				Body:    "¡Bienvenido {{ name }}! Gracias por unirte a nuestro centro de notificaciones.",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// System alert email - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "system_alert.email",
				Channel: "email",
				Locale:  "en",
				Subject: "System Alert: {{ title }}",
				Body:    "{{ message }}\n\nPlease check the dashboard for more details.",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// System alert in-app - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "system_alert.in-app",
				Channel: "in-app",
				Locale:  "en",
				Subject: "{{ title }}",
				Body:    "{{ message }}",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Comment reply in-app - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "comment_reply.in-app",
				Channel: "in-app",
				Locale:  "en",
				Subject: "New Reply",
				Body:    "{{ author }} replied to your comment: \"{{ message }}\"",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Comment reply in-app - Spanish
		{
			TemplateInput: templates.TemplateInput{
				Code:    "comment_reply.in-app",
				Channel: "in-app",
				Locale:  "es",
				Subject: "Nueva Respuesta",
				Body:    "{{ author }} respondió a tu comentario: \"{{ message }}\"",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Test email - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "test_notification.email",
				Channel: "email",
				Locale:  "en",
				Subject: "Test Notification",
				Body:    "Hello {{ name }},\n\n{{ message }}\n\nThis is a test of the multi-channel notification system.",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Test in-app - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "test_notification.in-app",
				Channel: "in-app",
				Locale:  "en",
				Subject: "Test Notification",
				Body:    "{{ message }}",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Admin message in-app - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "admin_message.in-app",
				Channel: "in-app",
				Locale:  "en",
				Subject: "Message from Admin",
				Body:    "{{ message }}",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Admin message chat - English (for Slack/Telegram)
		{
			TemplateInput: templates.TemplateInput{
				Code:    "admin_message.chat",
				Channel: "chat",
				Locale:  "en",
				Subject: "Admin Message",
				Body:    "📢 *Admin Message*\n{{ message }}",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Test chat - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "test_notification.chat",
				Channel: "chat",
				Locale:  "en",
				Subject: "Test Notification",
				Body:    "🧪 {{ message }}",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Test SMS - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "test_notification.sms",
				Channel: "sms",
				Locale:  "en",
				Subject: "",
				Body:    "{{ message }}",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// System alert SMS - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "system_alert.sms",
				Channel: "sms",
				Locale:  "en",
				Subject: "",
				Body:    "ALERT: {{ message }}",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// System alert Slack - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "system_alert.slack",
				Channel: "slack",
				Locale:  "en",
				Subject: "System Alert",
				Body:    "🚨 *{{ title }}*\n{{ message }}",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Welcome Slack - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "welcome.slack",
				Channel: "slack",
				Locale:  "en",
				Subject: "Welcome!",
				Body:    "👋 Welcome {{ name }}! Thanks for joining our notification center.",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Comment reply chat - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "comment_reply.chat",
				Channel: "chat",
				Locale:  "en",
				Subject: "New Reply",
				Body:    "💬 {{ author }} replied: \"{{ message }}\"",
				Format:  "text",
			},
			AllowUpdate: true,
		},
	}

	for _, tmpl := range templateData {
		if err := app.Catalog.SaveTemplate.Execute(ctx, tmpl); err != nil {
			// app.Logger.Error("failed to seed template", "code", tmpl.Code, "error", err)
		}
	}

	return nil
}

func seedContacts(ctx context.Context, app *App) error {
	if app.Directory == nil {
		return errors.New("seed: directory not configured")
	}
	phoneBook := map[string]string{
		"alice@example.com":  "+15555550100",
		"bob@example.com":    "+15555550200",
		"carlos@example.com": "+15555550300",
	}
	slackBook := map[string]string{
		"alice@example.com":  "U01ALICE",
		"bob@example.com":    "U02BOB",
		"carlos@example.com": "U03CARLOS",
	}
	telegramBook := map[string]string{
		"alice@example.com":  "10001",
		"bob@example.com":    "10002",
		"carlos@example.com": "10003",
	}

	for email, user := range app.Users {
		contact := demoContact{
			UserID:          user.ID,
			DisplayName:     user.Name,
			Email:           email,
			Phone:           phoneBook[email],
			SlackID:         slackBook[email],
			TelegramChatID:  telegramBook[email],
			PreferredLocale: user.Locale,
			TenantID:        user.TenantID,
		}
		if err := app.Directory.UpsertContact(ctx, contact); err != nil {
			return err
		}
	}
	return nil
}

func seedSecrets(ctx context.Context, app *App) error {
	if app.Directory == nil {
		return errors.New("seed: directory not configured")
	}
	defaults := []struct {
		ref   secrets.Reference
		value string
	}{
		{secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: "chat", Provider: "slack", Key: "default"}, "xoxb-system-demo-token"},
		{secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: "chat", Provider: "telegram", Key: "default"}, "telegram-system-demo-token"},
		{secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: "email", Provider: "sendgrid", Key: "api_key"}, "SG.system-demo-key"},
		{secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: "email", Provider: "sendgrid", Key: "from"}, "notifications@example.com"},
		{secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: "sms", Provider: "twilio", Key: "auth_token"}, "twilio-demo-token"},
		{secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: "sms", Provider: "twilio", Key: "from"}, "+15555550100"},
	}

	for _, entry := range defaults {
		if _, err := app.Directory.PutSecret(entry.ref, []byte(entry.value)); err != nil {
			return fmt.Errorf("seed: default secret %s/%s/%s: %w", entry.ref.Channel, entry.ref.Provider, entry.ref.Key, err)
		}
	}

	// Per-user chat tokens to demonstrate scoped resolution.
	for _, user := range app.Users {
		token := fmt.Sprintf("xoxb-%s-token", strings.ToLower(user.Name))
		telegramToken := fmt.Sprintf("telegram-%s-token", strings.ToLower(user.Name))
		entries := []struct {
			ref   secrets.Reference
			value string
		}{
			{secrets.Reference{Scope: secrets.ScopeUser, SubjectID: user.ID, Channel: "chat", Provider: "slack", Key: "token"}, token},
			{secrets.Reference{Scope: secrets.ScopeUser, SubjectID: user.ID, Channel: "chat", Provider: "slack", Key: "default"}, token},
			{secrets.Reference{Scope: secrets.ScopeUser, SubjectID: user.ID, Channel: "chat", Provider: "telegram", Key: "token"}, telegramToken},
			{secrets.Reference{Scope: secrets.ScopeUser, SubjectID: user.ID, Channel: "chat", Provider: "telegram", Key: "default"}, telegramToken},
		}
		for _, entry := range entries {
			if _, err := app.Directory.PutSecret(entry.ref, []byte(entry.value)); err != nil {
				return fmt.Errorf("seed: user secret %s/%s/%s: %w", entry.ref.Channel, entry.ref.Provider, entry.ref.Key, err)
			}
		}
	}
	return nil
}

func seedPreferences(ctx context.Context, app *App) error {
	// Seed some sample preferences for Bob (disable email for system_alert)
	bobUser := app.Users["bob@example.com"]
	if bobUser != nil {
		enabled := false
		_, err := app.Module.Preferences().Upsert(ctx, internalprefs.PreferenceInput{
			SubjectType:    "user",
			SubjectID:      bobUser.ID,
			DefinitionCode: "system_alert",
			Channel:        "email",
			Enabled:        &enabled,
		})
		if err != nil {
			// app.Logger.Error("failed to create preference", "error", err)
		}
	}

	// Disable comment_reply for Carlos
	carlosUser := app.Users["carlos@example.com"]
	if carlosUser != nil {
		enabled := false
		_, err := app.Module.Preferences().Upsert(ctx, internalprefs.PreferenceInput{
			SubjectType:    "user",
			SubjectID:      carlosUser.ID,
			DefinitionCode: "comment_reply",
			Channel:        "in-app",
			Enabled:        &enabled,
		})
		if err != nil {
			// app.Logger.Error("failed to create preference", "error", err)
		}
	}

	// Provider overrides for chat channel on test notification
	if bobUser != nil {
		provider := "slack"
		_, err := app.Module.Preferences().Upsert(ctx, internalprefs.PreferenceInput{
			SubjectType:    "user",
			SubjectID:      bobUser.ID,
			DefinitionCode: "test_notification",
			Channel:        "chat",
			Rules: domain.JSONMap{
				"channels": map[string]any{
					"chat": map[string]any{"provider": provider},
				},
			},
		})
		if err != nil {
			// app.Logger.Error("failed to set provider preference", "error", err)
		}
	}

	if carlosUser != nil {
		provider := "telegram"
		_, err := app.Module.Preferences().Upsert(ctx, internalprefs.PreferenceInput{
			SubjectType:    "user",
			SubjectID:      carlosUser.ID,
			DefinitionCode: "test_notification",
			Channel:        "chat",
			Rules: domain.JSONMap{
				"channels": map[string]any{
					"chat": map[string]any{"provider": provider},
				},
			},
		})
		if err != nil {
			// app.Logger.Error("failed to set provider preference", "error", err)
		}
	}

	return nil
}

func seedInboxItems(ctx context.Context, app *App) error {
	// Create some sample inbox items for demo users
	// 	for email, user := range app.Users {
	// 		if email == "alice@example.com" {
	// 			// Create welcome message
	// 			_, err := app.Module.Inbox().Create(ctx, &domain.InboxItem{
	// 				UserID:    user.ID,
	// 				MessageID: uuid.New(),
	// 				Title:     "Welcome to Notification Center!",
	// 				Body:      "Welcome " + user.Name + "! Thanks for joining our notification center.",
	// 				Locale:    user.Locale,
	// 				Unread:    true,
	// 				Pinned:    false,
	// 				ActionURL: "",
	// 				Metadata:  domain.JSONMap{"category": "welcome"},
	// 			})
	// 			if err != nil {
	// 				// app.Logger.Error("failed to create inbox item", "error", err)
	// 			}
	//
	// 			// Create a read notification
	// 			_, err = app.Module.Inbox().Create(ctx, &domain.InboxItem{
	// 				UserID:    user.ID,
	// 				MessageID: uuid.New(),
	// 				Title:     "System Update",
	// 				Body:      "System updated successfully",
	// 				Locale:    user.Locale,
	// 				Unread:    false,
	// 				Pinned:    false,
	// 				ReadAt:    time.Now().Add(-1 * time.Hour),
	// 				Metadata:  domain.JSONMap{"category": "system"},
	// 			})
	// 			if err != nil {
	// 				// app.Logger.Error("failed to create inbox item", "error", err)
	// 			}
	// 		}
	// 	}
	//
	return nil
}
