package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/goliatone/go-notifications/examples/web/config"
	"github.com/goliatone/go-router"
)

func main() {
	ctx := context.Background()

	cfg := config.Defaults()

	app, err := NewApp(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}
	defer app.Close()

	srv, err := buildServer(app)
	if err != nil {
		log.Fatalf("failed to configure server: %v", err)
	}

	app.SetupRoutes(srv.Router())
	registerWebSocketRoute(cfg, app, srv.Router())

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting server on %s", addr)
	log.Printf("Demo users:")
	for email, user := range app.Users {
		log.Printf("  - %s (%s) - locale: %s, admin: %v", email, user.Name, user.Locale, user.IsAdmin)
	}

	go func() {
		if err := srv.Serve(addr); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}

func buildServer(app *App) (router.Server[*fiber.App], error) {
	if logger := newRouterLogger(app.Logger); logger != nil {
		router.RegisterFiberWebSocketFactory(logger)
	}

	viewCfg := router.NewSimpleViewConfig("./views").
		WithExt(".html").
		WithReload(true).
		WithDebug(true)

	engine, err := router.InitializeViewEngine(viewCfg)
	if err != nil {
		return nil, err
	}

	srv := router.NewFiberAdapter(fiberAppInitializer(engine))

	return srv, nil
}

func fiberAppInitializer(engine fiber.Views) func(*fiber.App) *fiber.App {
	return func(*fiber.App) *fiber.App {
		app := fiber.New(fiber.Config{
			AppName:           "Notification Center Demo",
			PassLocalsToViews: true,
			Views:             engine,
		})

		app.Use(logger.New())
		app.Use(cors.New())

		return app
	}
}

func registerWebSocketRoute(cfg config.Config, app *App, r router.Router[*fiber.App]) {
	if !cfg.Features.EnableWebSocket || app.WSHub == nil {
		return
	}

	wsConfig := router.DefaultWebSocketConfig()
	wsConfig.Origins = []string{"*"}
	wsConfig.OnPreUpgrade = func(c router.Context) (router.UpgradeData, error) {
		userID := c.Query("user_id")
		wsLog("pre-upgrade request path=%s ip=%s user=%s", c.Path(), c.IP(), userID)
		if userID == "" {
			wsLog("pre-upgrade rejected path=%s ip=%s reason=missing-user", c.Path(), c.IP())
			return nil, fmt.Errorf("user_id query parameter required")
		}
		wsLog("pre-upgrade accepted path=%s ip=%s user=%s", c.Path(), c.IP(), userID)
		return router.UpgradeData{
			"user_id": userID,
		}, nil
	}
	r.WebSocket("/ws", wsConfig, app.HandleWebSocket)
}
