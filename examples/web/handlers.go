package main

import (
	"net/http"
	"time"

	"github.com/goliatone/go-notifications/internal/commands"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-router"
)

// Login handles user login.
func (a *App) Login(c router.Context) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid request"})
	}

	user := a.GetUserByEmail(req.Email)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "user not found"})
	}

	sessionID := a.CreateSession(user)
	c.Cookie(&router.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Expires:  time.Now().Add(a.Config.Auth.SessionTimeout),
		HTTPOnly: true,
		Path:     "/",
	})

	return c.JSON(http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":     user.ID,
			"name":   user.Name,
			"email":  user.Email,
			"locale": user.Locale,
			"admin":  user.IsAdmin,
		},
	})
}

// Logout handles user logout.
func (a *App) Logout(c router.Context) error {
	sessionID := c.Cookies(SessionCookieName)
	if sessionID != "" {
		a.DeleteSession(sessionID)
	}
	c.Cookie(&router.Cookie{
		Name:    SessionCookieName,
		Value:   "",
		Path:    "/",
		Expires: time.Now().Add(-1 * time.Hour),
	})
	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// CurrentUser returns the current user.
func (a *App) CurrentUser(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "not authenticated"})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"id":     user.ID,
		"name":   user.Name,
		"email":  user.Email,
		"locale": user.Locale,
		"admin":  user.IsAdmin,
	})
}

// ListInbox lists inbox items for the current user.
func (a *App) ListInbox(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	unreadOnly := c.Query("unread_only") == "true"
	filters := inbox.ListFilters{UnreadOnly: unreadOnly}

	opts := store.ListOptions{Limit: 20, Offset: 0}

	result, err := a.Module.Inbox().List(c.Context(), user.ID, opts, filters)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
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
func (a *App) MarkRead(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	id := c.Param("id", "")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "id required"})
	}

	err := a.Catalog.InboxMarkRead.Execute(c.Context(), commands.InboxMarkRead{
		UserID: user.ID,
		IDs:    []string{id},
		Read:   true,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// MarkUnread marks an inbox item as unread.
func (a *App) MarkUnread(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	id := c.Param("id", "")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "id required"})
	}

	err := a.Catalog.InboxMarkRead.Execute(c.Context(), commands.InboxMarkRead{
		UserID: user.ID,
		IDs:    []string{id},
		Read:   false,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// DismissNotification dismisses an inbox item.
func (a *App) DismissNotification(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	id := c.Param("id", "")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "id required"})
	}

	err := a.Catalog.InboxDismiss.Execute(c.Context(), commands.InboxDismiss{
		UserID: user.ID,
		ID:     id,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// SnoozeNotification snoozes an inbox item.
func (a *App) SnoozeNotification(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	id := c.Param("id", "")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "id required"})
	}

	var req struct {
		Until time.Time `json:"until"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid request"})
	}

	err := a.Catalog.InboxSnooze.Execute(c.Context(), commands.InboxSnooze{
		UserID: user.ID,
		ID:     id,
		Until:  req.Until,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// MarkAllRead marks all inbox items as read for the current user.
func (a *App) MarkAllRead(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	// Get all unread items
	result, err := a.Module.Inbox().List(c.Context(), user.ID, store.ListOptions{Limit: 1000, Offset: 0}, inbox.ListFilters{UnreadOnly: true})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	// Mark all as read
	ids := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		ids = append(ids, item.ID.String())
	}

	if len(ids) > 0 {
		err = a.Catalog.InboxMarkRead.Execute(c.Context(), commands.InboxMarkRead{
			UserID: user.ID,
			IDs:    ids,
			Read:   true,
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true, "count": len(ids)})
}

// InboxStats returns inbox statistics.
func (a *App) InboxStats(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	result, err := a.Module.Inbox().List(c.Context(), user.ID, store.ListOptions{Limit: 1000, Offset: 0}, inbox.ListFilters{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"unread": countUnread(result.Items),
		"total":  result.Total,
	})
}

// GetPreferences returns user preferences along with available definitions.
func (a *App) GetPreferences(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	// Get all definitions
	defs, err := a.Module.Container().Storage.Definitions.List(c.Context(), store.ListOptions{Limit: 100, Offset: 0})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	// Get user preferences
	result, err := a.Module.Preferences().List(c.Context(), store.ListOptions{Limit: 100, Offset: 0})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	// Build preference map for quick lookup
	prefMap := make(map[string]domain.NotificationPreference)
	for _, pref := range result.Items {
		if pref.SubjectID == user.ID {
			key := pref.DefinitionCode + ":" + pref.Channel
			prefMap[key] = pref
		}
	}

	// Build response with all definitions and their channels
	type PrefItem struct {
		DefinitionCode string `json:"definition_code"`
		DefinitionName string `json:"definition_name"`
		Channel        string `json:"channel"`
		Enabled        bool   `json:"enabled"`
		HasPreference  bool   `json:"has_preference"`
	}

	items := make([]PrefItem, 0)
	for _, def := range defs.Items {
		for _, channel := range def.Channels {
			key := def.Code + ":" + channel
			pref, exists := prefMap[key]
			items = append(items, PrefItem{
				DefinitionCode: def.Code,
				DefinitionName: def.Name,
				Channel:        channel,
				Enabled:        exists && pref.Enabled || !exists, // Default to enabled if no preference
				HasPreference:  exists,
			})
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"preferences": items})
}

// UpdatePreferences updates user preferences.
func (a *App) UpdatePreferences(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	var req preferences.PreferenceInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid request"})
	}

	req.SubjectID = user.ID
	req.SubjectType = "user"

	if err := a.Catalog.UpsertPreference.Execute(c.Context(), req); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// SendTestNotification sends a test notification.
func (a *App) SendTestNotification(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	if err := ensureSeededOrError(c.Context(), a); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
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
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// EnqueueEvent enqueues a custom event.
func (a *App) EnqueueEvent(c router.Context) error {
	user := GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}

	var req events.IntakeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid request"})
	}

	if err := a.Catalog.EnqueueEvent.Execute(c.Context(), req); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// CreateDefinition creates a notification definition (admin only).
func (a *App) CreateDefinition(c router.Context) error {
	var req commands.CreateDefinition
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid request"})
	}

	if err := a.Catalog.CreateDefinition.Execute(c.Context(), req); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// ListDefinitions lists all notification definitions.
func (a *App) ListDefinitions(c router.Context) error {
	result, err := a.Module.Container().Storage.Definitions.List(c.Context(), store.ListOptions{Limit: 1000, Offset: 0})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"definitions": result.Items})
}

// CreateTemplate creates a notification template (admin only).
func (a *App) CreateTemplate(c router.Context) error {
	var req commands.TemplateUpsert
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid request"})
	}

	if err := a.Catalog.SaveTemplate.Execute(c.Context(), req); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// ListTemplates lists all templates.
func (a *App) ListTemplates(c router.Context) error {
	result, err := a.Module.Container().Storage.Templates.List(c.Context(), store.ListOptions{Limit: 1000, Offset: 0})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"templates": result.Items})
}

// BroadcastNotification sends a notification to all users (admin only).
func (a *App) BroadcastNotification(c router.Context) error {
	var req struct {
		DefinitionCode string         `json:"definition_code"`
		Context        map[string]any `json:"context"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid request"})
	}

	recipients := make([]string, 0, len(a.Users))
	for _, user := range a.Users {
		recipients = append(recipients, user.ID)
	}

	if err := ensureSeededOrError(c.Context(), a); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	if err := a.Catalog.EnqueueEvent.Execute(c.Context(), events.IntakeRequest{
		DefinitionCode: req.DefinitionCode,
		Recipients:     recipients,
		Context:        req.Context,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true, "recipients": len(recipients)})
}

// ListUsers returns all demo users (admin only).
func (a *App) ListUsers(c router.Context) error {
	users := make([]map[string]any, 0, len(a.Users))
	for _, user := range a.Users {
		users = append(users, map[string]any{
			"id":     user.ID,
			"name":   user.Name,
			"email":  user.Email,
			"locale": user.Locale,
			"admin":  user.IsAdmin,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"users": users})
}

// SendToUser sends a notification to a specific user (admin only).
func (a *App) SendToUser(c router.Context) error {
	var req struct {
		UserID  string `json:"user_id"`
		Message string `json:"message"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid request"})
	}

	if req.UserID == "" || req.Message == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "user_id and message are required"})
	}

	if err := ensureSeededOrError(c.Context(), a); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	if err := a.Catalog.EnqueueEvent.Execute(c.Context(), events.IntakeRequest{
		DefinitionCode: "admin_message",
		Recipients:     []string{req.UserID},
		Context: map[string]any{
			"message": req.Message,
		},
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"success": true})
}

// GetAvailableChannels returns the list of configured notification channels.
func (a *App) GetAvailableChannels(c router.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"channels": a.AdapterRegistry.GetAvailableChannels(),
		"adapters": a.AdapterRegistry.EnabledAdapters,
	})
}

// DeliveryStats returns delivery statistics (admin only).
func (a *App) DeliveryStats(c router.Context) error {
	result, err := a.Module.Container().Storage.DeliveryAttempts.List(c.Context(), store.ListOptions{Limit: 10000, Offset: 0})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	succeeded := 0
	failed := 0
	for _, attempt := range result.Items {
		switch attempt.Status {
		case "succeeded":
			succeeded++
		case "failed":
			failed++
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"total":     len(result.Items),
		"succeeded": succeeded,
		"failed":    failed,
	})
}
