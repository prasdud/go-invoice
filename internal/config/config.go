package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	BaseURL              string
	Port                 int
	APIKey               string
	CompanyName          string
	CompanyRGB           [3]int
	CompanyLogo          string
	MaxConcurrentRenders int
	AdmissionWaitMS      int
	RenderTimeoutSeconds int
	LogLevel             string
	S3Endpoint           string
	S3Region             string
	S3Bucket             string
	S3AccessKey          string
	S3SecretKey          string
	S3UsePathStyle       bool
}

func Load() (*Config, error) {
	port, err := getEnvInt("PORT", 8080)
	if err != nil {
		return nil, err
	}
	maxRenders, err := getEnvInt("MAX_CONCURRENT_RENDERS", 16)
	if err != nil {
		return nil, err
	}
	admissionWait, err := getEnvInt("ADMISSION_WAIT_MS", 1000)
	if err != nil {
		return nil, err
	}
	renderTimeout, err := getEnvInt("RENDER_TIMEOUT_SECONDS", 10)
	if err != nil {
		return nil, err
	}
	rgb, err := parseColor(getEnv("COMPANY_COLOR", "284878"))
	if err != nil {
		return nil, fmt.Errorf("COMPANY_COLOR: %w", err)
	}

	return &Config{
		BaseURL:              getEnv("BASE_URL", "http://localhost:8080"),
		Port:                 port,
		APIKey:               getEnv("API_KEY", ""),
		CompanyName:          getEnv("COMPANY_NAME", "Invoice"),
		CompanyRGB:           rgb,
		CompanyLogo:          getEnv("COMPANY_LOGO", ""),
		MaxConcurrentRenders: maxRenders,
		AdmissionWaitMS:      admissionWait,
		RenderTimeoutSeconds: renderTimeout,
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		S3Endpoint:           getEnv("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:             getEnv("S3_REGION", "us-east-1"),
		S3Bucket:             getEnv("S3_BUCKET", "invoices"),
		S3AccessKey:          getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:          getEnv("S3_SECRET_KEY", "minioadmin"),
		S3UsePathStyle:       getEnvBool("S3_USE_PATH_STYLE", true),
	}, nil
}

func parseColor(s string) ([3]int, error) {
	var rgb [3]int
	if len(s) != 6 {
		return rgb, fmt.Errorf("expected 6 hex digits, got %q", s)
	}
	for i := range rgb {
		v, err := strconv.ParseUint(s[i*2:i*2+2], 16, 8)
		if err != nil {
			return rgb, fmt.Errorf("invalid hex %q", s)
		}
		rgb[i] = int(v)
	}
	return rgb, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return n, nil
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1"
}
