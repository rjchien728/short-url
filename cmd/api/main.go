package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/rjchien728/short-url/internal/handler"
	"github.com/rjchien728/short-url/internal/infra"
	"github.com/rjchien728/short-url/internal/pkg/logger"
	"github.com/rjchien728/short-url/internal/pkg/snowflake"
	"github.com/rjchien728/short-url/internal/repository/eventpub"
	"github.com/rjchien728/short-url/internal/repository/shorturl"
	"github.com/rjchien728/short-url/internal/repository/urlcache"
	redirectsvc "github.com/rjchien728/short-url/internal/service/redirect"
	urlsvc "github.com/rjchien728/short-url/internal/service/url"
)

func main() {
	ctx := context.Background()

	// --- Config ---
	cfg, err := infra.Load()
	if err != nil {
		logger.Error(ctx, "failed to load config", "error", err)
		os.Exit(1)
	}

	// --- Logger ---
	if err := logger.Setup(cfg.App.LogLevel, "text"); err != nil {
		logger.Warn(ctx, "logger setup failed, using defaults", "error", err)
	}

	// --- Infrastructure ---
	dbPool, err := infra.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Error(ctx, "failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	cacheRdb, err := infra.NewRedisClient(ctx, cfg.Cache)
	if err != nil {
		logger.Error(ctx, "failed to connect to redis cache", "error", err)
		os.Exit(1)
	}
	defer func() { _ = cacheRdb.Close() }()

	streamRdb, err := infra.NewRedisClient(ctx, cfg.Stream)
	if err != nil {
		logger.Error(ctx, "failed to connect to redis stream", "error", err)
		os.Exit(1)
	}
	defer func() { _ = streamRdb.Close() }()

	// --- Repository & Gateway ---
	urlRepo := shorturl.NewRepository(dbPool)
	cache := urlcache.NewCache(cacheRdb)
	publisher := eventpub.NewPublisher(streamRdb)
	idGen := snowflake.New(cfg.App.IDObfuscationSalt)

	// --- Service ---
	urlService := urlsvc.New(urlRepo, cache, publisher, idGen)
	redirectService := redirectsvc.New(urlRepo, cache, publisher)

	// --- HTTP Server ---
	e := echo.New()
	e.HideBanner = true
	e.Use(logger.Middleware())

	handler.RegisterHealthRoutes(e)
	handler.RegisterURLRoutes(e, urlService, cfg.Server.BaseURL)
	handler.RegisterRedirectRoutes(e, redirectService)

	// --- Graceful Shutdown ---
	go func() {
		if err := e.Start(":" + cfg.Server.Port); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info(ctx, "shutting down api server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Error(shutdownCtx, "server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info(ctx, "api server stopped")
}
