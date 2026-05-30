package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	BaseURL        string
	Port           int
	RedisURL       string
	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool
	SQLitePath     string
	APIKey         string
	AdminKey       string
}

func Load() (*Config, error) {
	port, err := getEnvInt("PORT", 8080)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		BaseURL:        getEnv("BASE_URL", "http://localhost:8080"),
		Port:           port,
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379"),
		S3Endpoint:     getEnv("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:       getEnv("S3_REGION", "us-east-1"),
		S3Bucket:       getEnv("S3_BUCKET", "invoices"),
		S3AccessKey:    getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:    getEnv("S3_SECRET_KEY", "minioadmin"),
		S3UsePathStyle: getEnvBool("S3_USE_PATH_STYLE", true),
		SQLitePath:     getEnv("SQLITE_PATH", "./data/invoices.db"),
		APIKey:         getEnv("API_KEY", ""),
		AdminKey:       getEnv("ADMIN_KEY", ""),
	}

	return cfg, nil
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
