package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	DB       DBConfig
	JWT      JWTConfig
	Redis    RedisConfig
	Firebase FirebaseConfig
	Twilio   TwilioConfig
	SendGrid SendGridConfig
	Tatum    TatumConfig
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

type TatumConfig struct {
	APIKey              string
	Env                 string
	IsMainnet           bool
	WebhookURL          string
	BTCChain            string
	ETHChain            string
	BTCMasterPrivateKey string
	ETHMasterPrivateKey string
}

type AppConfig struct {
	Port         string
	Env          string // "development" | "production"
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
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

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	TLS      bool
}

type FirebaseConfig struct {
	CredentialsJSON string
	ProjectID       string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading from environment")
	}

	return &Config{
		App: AppConfig{
			Port:         getEnv("APP_PORT", ":8080"),
			Env:          getEnv("APP_ENV", "development"),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
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
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
			TLS:      getEnvBool("REDIS_TLS", false),
		},
		Firebase: FirebaseConfig{
			CredentialsJSON: getEnv("FIREBASE_CREDENTIALS_JSON", ""),
			ProjectID:       getEnv("FIREBASE_PROJECT_ID", ""),
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
		Tatum: loadTatumConfig(),
	}
}

func loadTatumConfig() TatumConfig {
	env := mustGetEnv("TATUM_ENV")

	var apiKey, btcChain, ethChain string
	var isMainnet bool

	if env == "mainnet" {
		apiKey = mustGetEnv("TATUM_API_KEY_MAINNET")
		isMainnet = true
		btcChain = "BTC"
		ethChain = "ETH"
	} else if env == "testnet" {
		apiKey = mustGetEnv("TATUM_API_KEY_TESTNET")
		isMainnet = false
		btcChain = "BTC_TEST"
		ethChain = "ETH_SEPOLIA"
	} else {
		log.Fatalf("TATUM_ENV must be either 'mainnet' or 'testnet'")
	}

	return TatumConfig{
		APIKey:              apiKey,
		Env:                 env,
		IsMainnet:           isMainnet,
		WebhookURL:          mustGetEnv("TATUM_WEBHOOK_BASE_URL"),
		BTCChain:            btcChain,
		ETHChain:            ethChain,
		BTCMasterPrivateKey: mustGetEnv("BTC_MASTER_PRIVATE_KEY"),
		ETHMasterPrivateKey: mustGetEnv("ETH_MASTER_PRIVATE_KEY"),
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
