package main

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-notifications/internal/commands"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/preferences"
)

// Login handles user login.
func (a *App) Login(c *fiber.Ctx) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	user := a.GetUserByEmail(req.Email)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "user not found"})
	}

	sessionID := a.CreateSession(user)
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Expires:  time.Now().Add(a.Config.Auth.SessionTimeout),
		HTTPOnly: true,
	})

	return c.JSON(fiber.Map{
		"user": fiber.Map{
			"id":     user.ID,
			"name":   user.Name,
			"email":  user.Email,
			"locale": user.Locale,
			"admin":  user.IsAdmin,
		},
	})
}

// Logout handles user logout.
func (a *App) Logout(c *fiber.Ctx) error {
	sessionID := c.Cookies(SessionCookieName)
	if sessionID != "" {
		a.DeleteSession(sessionID)
	}
	c.ClearCookie(SessionCookieName)
	return c.JSON(fiber.Map{"success": true})
}

// CurrentUser returns the current user.
func (a *App) CurrentUser(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "not authenticated"})
	}
	return c.JSON(fiber.Map{
		"id":     user.ID,
		"name":   user.Name,
		"email":  user.Email,
		"locale": user.Locale,
		"admin":  user.IsAdmin,
	})
}

// ListInbox lists inbox items for the current user.
func (a *App) ListInbox(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	unreadOnly := c.Query("unread_only") == "true"

	filters := inbox.ListFilters{
		UnreadOnly: unreadOnly,
	}

	opts := store.ListOptions{
		Limit:  20,
		Offset: 0,
	}

	result, err := a.Module.Inbox().List(c.Context(), user.ID, opts, filters)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"items":        result.Items,
		"total":        result.Total,
		"limit":        opts.Limit,
		"unread_count": countUnread(result.Items),
	})
}

func countUnread(items []domain.InboxItem) int {
	count := 0
	for _, item := range items {
		if item.Unread {
			count++
		}
	}
	return count
}

// MarkRead marks an inbox item as read.
func (a *App) MarkRead(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id required"})
	}

	err := a.Catalog.InboxMarkRead.Execute(c.Context(), commands.InboxMarkRead{
		UserID: user.ID,
		IDs:    []string{id},
		Read:   true,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// MarkUnread marks an inbox item as unread.
func (a *App) MarkUnread(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id required"})
	}

	err := a.Catalog.InboxMarkRead.Execute(c.Context(), commands.InboxMarkRead{
		UserID: user.ID,
		IDs:    []string{id},
		Read:   false,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// DismissNotification dismisses an inbox item.
func (a *App) DismissNotification(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id required"})
	}

	err := a.Catalog.InboxDismiss.Execute(c.Context(), commands.InboxDismiss{
		UserID: user.ID,
		ID:     id,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// SnoozeNotification snoozes an inbox item.
func (a *App) SnoozeNotification(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id required"})
	}

	var req struct {
		Until time.Time `json:"until"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	err := a.Catalog.InboxSnooze.Execute(c.Context(), commands.InboxSnooze{
		UserID: user.ID,
		ID:     id,
		Until:  req.Until,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// InboxStats returns inbox statistics.
func (a *App) InboxStats(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// Get inbox items to calculate stats
	result, err := a.Module.Inbox().List(c.Context(), user.ID, store.ListOptions{Limit: 1000, Offset: 0}, inbox.ListFilters{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"unread": countUnread(result.Items),
		"total":  result.Total,
	})
}

// GetPreferences returns user preferences.
func (a *App) GetPreferences(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	result, err := a.Module.Preferences().List(c.Context(), store.ListOptions{Limit: 100, Offset: 0})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Filter by user
	userPrefs := []domain.NotificationPreference{}
	for _, pref := range result.Items {
		if pref.SubjectID == user.ID {
			userPrefs = append(userPrefs, pref)
		}
	}

	return c.JSON(fiber.Map{"preferences": userPrefs})
}

// UpdatePreferences updates user preferences.
func (a *App) UpdatePreferences(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req preferences.PreferenceInput
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	req.SubjectID = user.ID
	req.SubjectType = "user"

	err := a.Catalog.UpsertPreference.Execute(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// SendTestNotification sends a test notification.
func (a *App) SendTestNotification(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	err := a.Catalog.EnqueueEvent.Execute(c.Context(), events.IntakeRequest{
		DefinitionCode: "test_notification",
		Recipients:     []string{user.ID},
		Context: map[string]any{
			"name":    user.Name,
			"message": "This is a test notification",
		},
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// EnqueueEvent enqueues a custom event.
func (a *App) EnqueueEvent(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req events.IntakeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	err := a.Catalog.EnqueueEvent.Execute(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// CreateDefinition creates a notification definition (admin only).
func (a *App) CreateDefinition(c *fiber.Ctx) error {
	var req commands.CreateDefinition
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	err := a.Catalog.CreateDefinition.Execute(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// ListDefinitions lists all notification definitions.
func (a *App) ListDefinitions(c *fiber.Ctx) error {
	result, err := a.Module.Container().Storage.Definitions.List(c.Context(), store.ListOptions{Limit: 1000, Offset: 0})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"definitions": result.Items})
}

// CreateTemplate creates a notification template (admin only).
func (a *App) CreateTemplate(c *fiber.Ctx) error {
	var req commands.TemplateUpsert
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	err := a.Catalog.SaveTemplate.Execute(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// ListTemplates lists all templates.
func (a *App) ListTemplates(c *fiber.Ctx) error {
	result, err := a.Module.Container().Storage.Templates.List(c.Context(), store.ListOptions{Limit: 1000, Offset: 0})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"templates": result.Items})
}

// BroadcastNotification sends a notification to all users (admin only).
func (a *App) BroadcastNotification(c *fiber.Ctx) error {
	var req struct {
		DefinitionCode string         `json:"definition_code"`
		Context        map[string]any `json:"context"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	recipients := make([]string, 0, len(a.Users))
	for _, user := range a.Users {
		recipients = append(recipients, user.ID)
	}

	err := a.Catalog.EnqueueEvent.Execute(c.Context(), events.IntakeRequest{
		DefinitionCode: req.DefinitionCode,
		Recipients:     recipients,
		Context:        req.Context,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "recipients": len(recipients)})
}

// DeliveryStats returns delivery statistics (admin only).
func (a *App) DeliveryStats(c *fiber.Ctx) error {
	result, err := a.Module.Container().Storage.DeliveryAttempts.List(c.Context(), store.ListOptions{Limit: 10000, Offset: 0})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	succeeded := 0
	failed := 0
	for _, attempt := range result.Items {
		if attempt.Status == "succeeded" {
			succeeded++
		} else if attempt.Status == "failed" {
			failed++
		}
	}

	return c.JSON(fiber.Map{
		"total":     len(result.Items),
		"succeeded": succeeded,
		"failed":    failed,
	})
}
