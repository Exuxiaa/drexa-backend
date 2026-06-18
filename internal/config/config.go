package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	DB       DBConfig
	JWT      JWTConfig
	Twilio   TwilioConfig
	SendGrid SendGridConfig
	Tatum    TatumConfig
	Stripe   StripeConfig
}

type StripeConfig struct {
	SecretKey     string
	WebhookSecret string
}

type TatumConfig struct {
	APIKey  string // active key, chosen from TATUM_ENV
	BaseURL string
	Testnet bool
}

type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	FromPhone  string
}

type SendGridConfig struct {
	APIKey    string
	FromEmail string
	FromName  string
	AppURL    string
}

type AppConfig struct {
	Port           string
	Env            string // "development" | "production"
	AllowedOrigins []string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
}

type DBConfig struct {
	DSN             string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

type JWTConfig struct {
	Secret     string
	Expiration time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading from environment")
	}

	// Tatum: pick the testnet or mainnet key based on TATUM_ENV.
	tatumEnv := getEnv("TATUM_ENV", "testnet")
	tatumKey := getEnv("TATUM_TESTNET_API_KEY", "")
	if tatumEnv == "mainnet" {
		tatumKey = getEnv("TATUM_WALLET_API_KEY", "")
	}

	return &Config{
		App: AppConfig{
			Port:           getEnv("APP_PORT", ":8080"),
			Env:            getEnv("APP_ENV", "development"),
			AllowedOrigins: getEnvCSV("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   10 * time.Second,
			IdleTimeout:    120 * time.Second,
		},
		DB: DBConfig{
			DSN:             mustGetEnv("DB_DSN"),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 100),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", time.Hour),
		},
		JWT: JWTConfig{
			Secret:     mustGetEnv("JWT_SECRET"),
			Expiration: getEnvDuration("JWT_EXPIRATION", 24*time.Hour),
		},
		Twilio: TwilioConfig{
			AccountSID: getEnv("TWILIO_ACCOUNT_SID", ""),
			AuthToken:  getEnv("TWILIO_AUTH_TOKEN", ""),
			FromPhone:  getEnv("TWILIO_FROM_PHONE", ""),
		},
		SendGrid: SendGridConfig{
			APIKey:    getEnv("SENDGRID_API_KEY", ""),
			FromEmail: getEnv("SENDGRID_FROM_EMAIL", ""),
			FromName:  getEnv("SENDGRID_FROM_NAME", "Drexa"),
			AppURL:    getEnv("APP_URL", "http://localhost:3000"),
		},
		Tatum: TatumConfig{
			APIKey:  tatumKey,
			BaseURL: getEnv("TATUM_BASE_URL", "https://api.tatum.io"),
			Testnet: tatumEnv != "mainnet",
		},
		Stripe: StripeConfig{
			SecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
			WebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		},
	}
}

// required — panics if missing
func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required environment variable: %s", key)
	}
	return v
}

// optional — returns fallback if missing
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// optional — splits a comma-separated env var into a trimmed slice
func getEnvCSV(key, fallback string) []string {
	v := os.Getenv(key)
	if v == "" {
		v = fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
