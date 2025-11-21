package main

import (
	"sort"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-router"
)

// SetupRoutes configures all HTTP routes.
func (a *App) SetupRoutes(r router.Router[*fiber.App]) {
	// Static assets for JS/CSS.
	r.Static("/public", "./public")

	r.Get("/", a.renderHome())
	r.Post("/auth/login", a.Login)
	r.Post("/auth/logout", a.Logout)

	api := r.Group("/api")
	api.Use(a.AuthMiddleware())

	api.Get("/user", a.CurrentUser)

	api.Get("/inbox", a.ListInbox)
	api.Post("/inbox/:id/read", a.MarkRead)
	api.Post("/inbox/:id/unread", a.MarkUnread)
	api.Post("/inbox/:id/dismiss", a.DismissNotification)
	api.Post("/inbox/:id/snooze", a.SnoozeNotification)
	api.Post("/inbox/mark-all-read", a.MarkAllRead)
	api.Get("/inbox/stats", a.InboxStats)

	api.Get("/preferences", a.GetPreferences)
	api.Put("/preferences", a.UpdatePreferences)

	api.Get("/channels", a.GetAvailableChannels)

	api.Post("/notify/test", a.SendTestNotification)
	api.Post("/notify/event", a.EnqueueEvent)

	admin := r.Group("/admin")
	admin.Use(a.AuthMiddleware(), a.AdminMiddleware())

	admin.Post("/definitions", a.CreateDefinition)
	admin.Get("/definitions", a.ListDefinitions)
	admin.Post("/templates", a.CreateTemplate)
	admin.Get("/templates", a.ListTemplates)
	admin.Post("/broadcast", a.BroadcastNotification)
	admin.Get("/stats", a.DeliveryStats)
	admin.Get("/users", a.ListUsers)
	admin.Post("/send-to-user", a.SendToUser)
}

func (a *App) renderHome() router.HandlerFunc {
	return func(c router.Context) error {
		users := make([]*DemoUser, 0, len(a.Users))
		for _, user := range a.Users {
			users = append(users, user)
		}
		sort.Slice(users, func(i, j int) bool {
			if users[i].Name == users[j].Name {
				return users[i].Email < users[j].Email
			}
			return users[i].Name < users[j].Name
		})
		return c.Render("home", router.ViewContext{
			"title":        "Notification Center Demo",
			"description":  "Test the go-notifications module end-to-end",
			"demo_users":   users,
			"ws_enabled":   a.Config.Features.EnableWebSocket,
			"feature_set":  a.Config.Features,
			"server_host":  a.Config.Server.Host,
			"server_port":  a.Config.Server.Port,
			"default_user": "alice@example.com",
		})
	}
}
