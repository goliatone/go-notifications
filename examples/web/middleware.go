package main

import (
	"github.com/gofiber/fiber/v2"
)

const (
	SessionCookieName = "session_id"
	UserContextKey    = "user"
)

// AuthMiddleware checks for valid session.
func (a *App) AuthMiddleware(c *fiber.Ctx) error {
	sessionID := c.Cookies(SessionCookieName)
	if sessionID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	user := a.GetUserBySession(sessionID)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "invalid session",
		})
	}

	c.Locals(UserContextKey, user)
	return c.Next()
}

// AdminMiddleware checks if user is admin.
func (a *App) AdminMiddleware(c *fiber.Ctx) error {
	user := GetUser(c)
	if user == nil || !user.IsAdmin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "admin access required",
		})
	}
	return c.Next()
}

// GetUser retrieves the current user from context.
func GetUser(c *fiber.Ctx) *DemoUser {
	user, ok := c.Locals(UserContextKey).(*DemoUser)
	if !ok {
		return nil
	}
	return user
}
