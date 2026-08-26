package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL      string
	RedisURL         string
	JWTSecret        string
	Port             string
	Env              string // "development" | "production" — dipakai buat flag Secure di cookie
	FrontendURL      string // dipakai buat CORS whitelist (06-ROADMAP.md §6 Security Checklist)
	ResendAPIKey     string
	EmailFromAddress string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		Port:             os.Getenv("PORT"),
		Env:              getEnvDefault("APP_ENV", "development"),
		FrontendURL:      getEnvDefault("FRONTEND_URL", "http://localhost:3000"),
		ResendAPIKey:     os.Getenv("RESEND_API_KEY"),
		EmailFromAddress: getEnvDefault("EMAIL_FROM_ADDRESS", "onboarding@resend.dev"),
	}

	if cfg.DatabaseURL == "" {
		return nil, errMissingEnv("DATABASE_URL")
	}
	if cfg.JWTSecret == "" {
		return nil, errMissingEnv("JWT_SECRET")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	return cfg, nil
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func errMissingEnv(key string) error {
	return &missingEnvError{key: key}
}

type missingEnvError struct{ key string }

func (e *missingEnvError) Error() string {
	return "missing required env var: " + e.key
}
