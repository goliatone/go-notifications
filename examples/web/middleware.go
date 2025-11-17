package main

import (
	"net/http"

	"github.com/goliatone/go-router"
)

const (
	SessionCookieName = "session_id"
	UserContextKey    = "user"
)

// AuthMiddleware ensures the request is authenticated.
func (a *App) AuthMiddleware() router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			sessionID := c.Cookies(SessionCookieName)
			if sessionID == "" {
				return c.JSON(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			}

			user := a.GetUserBySession(sessionID)
			if user == nil {
				return c.JSON(http.StatusUnauthorized, map[string]any{"error": "invalid session"})
			}

			c.Locals(UserContextKey, user)
			return next(c)
		}
	}
}

// AdminMiddleware ensures the current user has admin privileges.
func (a *App) AdminMiddleware() router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			user := GetUser(c)
			if user == nil || !user.IsAdmin {
				return c.JSON(http.StatusForbidden, map[string]any{"error": "admin access required"})
			}
			return next(c)
		}
	}
}

// GetUser retrieves the current user from context.
func GetUser(c router.Context) *DemoUser {
	user, ok := c.Locals(UserContextKey).(*DemoUser)
	if !ok {
		return nil
	}
	return user
}
