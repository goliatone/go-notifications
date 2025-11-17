package main

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/google/uuid"
)

// SetupRoutes configures all HTTP routes.
func (a *App) SetupRoutes(app *fiber.App) {
	// Middleware
	app.Use(logger.New())
	app.Use(cors.New())

	// Public routes
	app.Post("/auth/login", a.Login)
	app.Post("/auth/logout", a.Logout)

	// Static files
	app.Static("/public", "./examples/web/public")
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("./examples/web/public/index.html")
	})

	// API routes (protected)
	api := app.Group("/api", a.AuthMiddleware)

	// User info
	api.Get("/user", a.CurrentUser)

	// Inbox
	api.Get("/inbox", a.ListInbox)
	api.Post("/inbox/:id/read", a.MarkRead)
	api.Post("/inbox/:id/unread", a.MarkUnread)
	api.Post("/inbox/:id/dismiss", a.DismissNotification)
	api.Post("/inbox/:id/snooze", a.SnoozeNotification)
	api.Get("/inbox/stats", a.InboxStats)

	// Preferences
	api.Get("/preferences", a.GetPreferences)
	api.Put("/preferences", a.UpdatePreferences)

	// Notifications
	api.Post("/notify/test", a.SendTestNotification)
	api.Post("/notify/event", a.EnqueueEvent)

	// Admin routes
	admin := app.Group("/admin", a.AuthMiddleware, a.AdminMiddleware)
	admin.Post("/definitions", a.CreateDefinition)
	admin.Get("/definitions", a.ListDefinitions)
	admin.Post("/templates", a.CreateTemplate)
	admin.Get("/templates", a.ListTemplates)
	admin.Post("/broadcast", a.BroadcastNotification)
	admin.Get("/stats", a.DeliveryStats)

	// WebSocket
	if a.Config.Features.EnableWebSocket && a.WSHub != nil {
		app.Get("/ws", websocket.New(a.HandleWebSocket))
	}
}

// HandleWebSocket handles WebSocket connections.
func (a *App) HandleWebSocket(c *websocket.Conn) {
	// Get user from query params (in production, use proper auth)
	userID := c.Query("user_id")
	if userID == "" {
		c.Close()
		return
	}

	client := &WebSocketClient{
		ID:     uuid.New().String(),
		UserID: userID,
		Conn:   c,
		Send:   make(chan []byte, 256),
		hub:    a.WSHub,
	}

	a.WSHub.RegisterClient(client)
	client.HandleConnection()
}
