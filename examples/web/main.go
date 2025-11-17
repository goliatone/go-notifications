package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-notifications/examples/web/config"
)

func main() {
	ctx := context.Background()

	// Load configuration
	cfg := config.Defaults()

	// Create app
	app, err := NewApp(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}
	defer app.Close()

	// Create Fiber app
	fiberApp := fiber.New(fiber.Config{
		AppName: "Notification Center Demo",
	})

	// Setup routes
	app.SetupRoutes(fiberApp)

	// Start server
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting server on %s", addr)
	log.Printf("Demo users:")
	for email, user := range app.Users {
		log.Printf("  - %s (%s) - locale: %s, admin: %v", email, user.Name, user.Locale, user.IsAdmin)
	}

	go func() {
		if err := fiberApp.Listen(addr); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	if err := fiberApp.Shutdown(); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}
