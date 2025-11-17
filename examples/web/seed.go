package main

import (
	"context"

	"github.com/goliatone/go-notifications/internal/commands"
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
	if err := seedInboxItems(ctx, app); err != nil {
		return err
	}
	return nil
}

func seedDefinitions(ctx context.Context, app *App) error {
	definitions := []commands.CreateDefinition{
		{
			Code:        "welcome",
			Name:        "Welcome Message",
			Description: "Welcome new users",
			Severity:    "info",
			Category:    "onboarding",
			Channels:    []string{"email", "in-app"},
			TemplateIDs: []string{"welcome.email", "welcome.in-app"},
			AllowUpdate: true,
		},
		{
			Code:        "comment_reply",
			Name:        "Comment Reply",
			Description: "Notify when someone replies to a comment",
			Severity:    "info",
			Category:    "social",
			Channels:    []string{"in-app"},
			TemplateIDs: []string{"comment_reply.in-app"},
			AllowUpdate: true,
		},
		{
			Code:        "system_alert",
			Name:        "System Alert",
			Description: "Critical system notifications",
			Severity:    "critical",
			Category:    "system",
			Channels:    []string{"email", "in-app"},
			TemplateIDs: []string{"system_alert.email", "system_alert.in-app"},
			AllowUpdate: true,
		},
		{
			Code:        "test_notification",
			Name:        "Test Notification",
			Description: "For testing multi-channel delivery",
			Severity:    "info",
			Category:    "test",
			Channels:    []string{"email", "in-app"},
			TemplateIDs: []string{"test.email", "test.in-app"},
			AllowUpdate: true,
		},
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
				Body:    "Hello {{ .name }},\n\nWelcome to our notification system. You can manage your preferences at any time.\n\nBest regards,\nThe Team",
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
				Body:    "Hola {{ .name }},\n\n¡Bienvenido a nuestro sistema de notificaciones! Puedes gestionar tus preferencias en cualquier momento.\n\nSaludos,\nEl Equipo",
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
				Body:    "Welcome {{ .name }}! Thanks for joining our notification center.",
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
				Body:    "¡Bienvenido {{ .name }}! Gracias por unirte a nuestro centro de notificaciones.",
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
				Subject: "System Alert: {{ .title }}",
				Body:    "{{ .message }}\n\nPlease check the dashboard for more details.",
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
				Subject: "{{ .title }}",
				Body:    "{{ .message }}",
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
				Body:    "{{ .author }} replied to your comment: \"{{ .message }}\"",
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
				Body:    "{{ .author }} respondió a tu comentario: \"{{ .message }}\"",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Test email - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "test.email",
				Channel: "email",
				Locale:  "en",
				Subject: "Test Notification",
				Body:    "Hello {{ .name }},\n\n{{ .message }}\n\nThis is a test of the multi-channel notification system.",
				Format:  "text",
			},
			AllowUpdate: true,
		},
		// Test in-app - English
		{
			TemplateInput: templates.TemplateInput{
				Code:    "test.in-app",
				Channel: "in-app",
				Locale:  "en",
				Subject: "Test Notification",
				Body:    "{{ .message }}",
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
