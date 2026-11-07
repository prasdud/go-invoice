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
	"strings"
	"syscall"
	"time"

	"github.com/prasdud/go-invoice/internal/api"
	"github.com/prasdud/go-invoice/internal/config"
	"github.com/prasdud/go-invoice/internal/renderer"
	"github.com/prasdud/go-invoice/internal/renderer/templates"
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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))
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

	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 5*time.Second)
	if err := store.EnsureBucket(bootCtx); err != nil {
		logger.Warn("bucket bootstrap failed", "error", err)
	}
	cancelBoot()

	var logo *renderer.Logo
	if cfg.CompanyLogo != "" {
		logo, err = renderer.LoadLogo(cfg.CompanyLogo)
		if err != nil {
			log.Fatalf("logo: %v", err)
		}
		logger.Info("logo loaded", "path", cfg.CompanyLogo, "type", logo.Type, "bytes", len(logo.Bytes))
	}

	theme, ok := templates.Get(cfg.Template)
	if !ok {
		log.Fatalf("config: unknown TEMPLATE %q", cfg.Template)
	}
	logger.Info("theme loaded", "name", theme.Name)

	srv := api.New(api.Config{
		BaseURL:              cfg.BaseURL,
		APIKey:               cfg.APIKey,
		CompanyName:          cfg.CompanyName,
		CompanyRGB:           cfg.CompanyRGB,
		Logo:                 logo,
		Theme:                theme,
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

func logLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
