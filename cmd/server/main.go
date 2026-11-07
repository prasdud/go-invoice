package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prasdud/go-invoice/internal/api"
	"github.com/prasdud/go-invoice/internal/config"
	"github.com/prasdud/go-invoice/internal/renderer"
	"github.com/prasdud/go-invoice/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.APIKey == "" {
		log.Fatal("config: API_KEY must be set")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	store, err := storage.New(storage.Config{
		Endpoint:     cfg.S3Endpoint,
		Region:       cfg.S3Region,
		Bucket:       cfg.S3Bucket,
		AccessKey:    cfg.S3AccessKey,
		SecretKey:    cfg.S3SecretKey,
		UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	srv := api.New(api.Config{
		BaseURL:              cfg.BaseURL,
		APIKey:               cfg.APIKey,
		CompanyName:          cfg.CompanyName,
		CompanyRGB:           cfg.CompanyRGB,
		MaxConcurrentRenders: cfg.MaxConcurrentRenders,
		AdmissionWait:        time.Duration(cfg.AdmissionWaitMS) * time.Millisecond,
		RenderTimeout:        time.Duration(cfg.RenderTimeoutSeconds) * time.Second,
	}, renderer.Generate, store, logger)

	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	logger.Info("stopped")
}
