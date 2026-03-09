package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	clickconsumer "github.com/rjchien728/short-url/internal/consumer/click"
	ogconsumer "github.com/rjchien728/short-url/internal/consumer/og"
	"github.com/rjchien728/short-url/internal/gateway/ogfetch"
	"github.com/rjchien728/short-url/internal/infra"
	"github.com/rjchien728/short-url/internal/pkg/logger"
	"github.com/rjchien728/short-url/internal/repository/clicklog"
	"github.com/rjchien728/short-url/internal/repository/eventpub"
	"github.com/rjchien728/short-url/internal/repository/shorturl"
	clickworkersvc "github.com/rjchien728/short-url/internal/service/clickworker"
	ogworkersvc "github.com/rjchien728/short-url/internal/service/ogworker"
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
	// Worker only needs the stream Redis client (no cache Redis needed).
	dbPool, err := infra.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Error(ctx, "failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	streamRdb, err := infra.NewRedisClient(ctx, cfg.Stream)
	if err != nil {
		logger.Error(ctx, "failed to connect to redis stream", "error", err)
		os.Exit(1)
	}
	defer func() { _ = streamRdb.Close() }()

	// --- Repository & Gateway ---
	urlRepo := shorturl.NewRepository(dbPool)
	clickRepo := clicklog.NewRepository(dbPool)
	publisher := eventpub.NewPublisher(streamRdb)
	fetcher := ogfetch.NewFetcher(&http.Client{Timeout: 10 * time.Second}, cfg.App.OGDefaultImage)

	// --- Service ---
	ogSvc := ogworkersvc.New(urlRepo, fetcher, publisher)
	clickSvc := clickworkersvc.New(clickRepo)

	// --- Consumer ---
	consumerCfg := cfg.Consumer
	ogC := ogconsumer.New(
		streamRdb, ogSvc,
		consumerCfg.OGGroupName,
		consumerCfg.ConsumerName,
	)
	clickC := clickconsumer.New(
		streamRdb, clickSvc,
		consumerCfg.ClickGroupName,
		consumerCfg.ConsumerName,
		consumerCfg.ClickBatchSize,
		consumerCfg.MaxDelivery,
	)

	// --- Graceful Shutdown ---
	workerCtx, cancel := context.WithCancel(ctx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		logger.Info(ctx, "shutting down worker...")
		cancel()
	}()

	// --- Run consumers with errgroup ---
	g, gCtx := errgroup.WithContext(workerCtx)
	g.Go(func() error { return ogC.Run(gCtx) })
	g.Go(func() error { return clickC.Run(gCtx) })

	if err := g.Wait(); err != nil {
		logger.Error(gCtx, "worker stopped with error", "error", err)
		os.Exit(1)
	}

	logger.Info(ctx, "worker stopped")
}
