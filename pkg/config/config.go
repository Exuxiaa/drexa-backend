package config

import (
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// splitCSV turns a comma-separated env value into a trimmed, non-empty slice.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

type Config struct {
	App      AppConfig
	DB       DBConfig
	JWT      JWTConfig
	Twilio   TwilioConfig
	SendGrid SendGridConfig
	Tatum    TatumConfig
	Stripe   StripeConfig
	Didit    DiditConfig
	Google   GoogleConfig
}

type GoogleConfig struct {
	ClientID string
}

type AppConfig struct {
	Port           string
	Env            string
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
	Secret            string
	AccessExpiration  time.Duration
	RefreshExpiration time.Duration
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
	APIKey        string
	BTCGatewayURL string
	ETHGatewayURL string
	BTCAddress    string
	BTCPrivateKey string
	ETHPrivateKey string
}

type StripeConfig struct {
	SecretKey      string
	WebhookSecret  string
	PublishableKey string
}

// DiditConfig holds Didit identity-verification credentials.
// WorkflowID is per-session config (not a secret) — kept here for convenience.
type DiditConfig struct {
	APIKey        string
	WebhookSecret string
	WorkflowID    string
}

func Load() *Config {
	_ = godotenv.Load() // optional; env vars take precedence
	viper.AutomaticEnv()

	viper.SetDefault("APP_PORT", ":8080")
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("DB_MAX_IDLE_CONNS", 10)
	viper.SetDefault("DB_MAX_OPEN_CONNS", 100)
	viper.SetDefault("DB_CONN_MAX_LIFETIME", "1h")
	viper.SetDefault("JWT_ACCESS_EXPIRATION", "15m")
	viper.SetDefault("JWT_REFRESH_EXPIRATION", "168h")
	viper.SetDefault("SENDGRID_FROM_NAME", "Drexa")
	viper.SetDefault("APP_URL", "http://localhost:3000")
	// Didit "Drexa" KYC workflow. Per-session config, not a secret — overridable via env.
	viper.SetDefault("DIDIT_WORKFLOW_ID", "3b3ef226-0f3f-49cb-9be6-9fbfc19a0885")

	return &Config{
		App: AppConfig{
			Port:           viper.GetString("APP_PORT"),
			Env:            viper.GetString("APP_ENV"),
			AllowedOrigins: splitCSV(viper.GetString("CORS_ALLOWED_ORIGINS")),
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   10 * time.Second,
			IdleTimeout:    120 * time.Second,
		},
		DB: DBConfig{
			DSN:             viper.GetString("DB_DSN"),
			MaxIdleConns:    viper.GetInt("DB_MAX_IDLE_CONNS"),
			MaxOpenConns:    viper.GetInt("DB_MAX_OPEN_CONNS"),
			ConnMaxLifetime: viper.GetDuration("DB_CONN_MAX_LIFETIME"),
		},
		JWT: JWTConfig{
			Secret:            viper.GetString("JWT_SECRET"),
			AccessExpiration:  viper.GetDuration("JWT_ACCESS_EXPIRATION"),
			RefreshExpiration: viper.GetDuration("JWT_REFRESH_EXPIRATION"),
		},
		Twilio: TwilioConfig{
			AccountSID: viper.GetString("TWILIO_ACCOUNT_SID"),
			AuthToken:  viper.GetString("TWILIO_AUTH_TOKEN"),
			FromPhone:  viper.GetString("TWILIO_FROM_PHONE"),
		},
		SendGrid: SendGridConfig{
			APIKey:    viper.GetString("SENDGRID_API_KEY"),
			FromEmail: viper.GetString("SENDGRID_FROM_EMAIL"),
			FromName:  viper.GetString("SENDGRID_FROM_NAME"),
			AppURL:    viper.GetString("APP_URL"),
		},
		Tatum: TatumConfig{
			APIKey:        viper.GetString("TATUM_TESTNET_API_KEY"),
			BTCGatewayURL: viper.GetString("TATUM_BTC_GATEWAY_URL"),
			ETHGatewayURL: viper.GetString("TATUM_ETH_GATEWAY_URL"),
			BTCAddress:    viper.GetString("BTC_MASTER_ADDRESS"),
			BTCPrivateKey: viper.GetString("BTC_MASTER_PRIVATE_KEY"),
			ETHPrivateKey: viper.GetString("ETH_MASTER_PRIVATE_KEY"),
		},
		Stripe: StripeConfig{
			SecretKey:      viper.GetString("STRIPE_SECRET_KEY"),
			WebhookSecret:  viper.GetString("STRIPE_WEBHOOK_SECRET"),
			PublishableKey: viper.GetString("STRIPE_PUBLISHABLE_KEY"),
		},
		Didit: DiditConfig{
			APIKey:        viper.GetString("DIDIT_API_KEY"),
			WebhookSecret: viper.GetString("DIDIT_WEBHOOK_SECRET"),
			WorkflowID:    viper.GetString("DIDIT_WORKFLOW_ID"),
		Google: GoogleConfig{
			ClientID: viper.GetString("GOOGLE_CLIENT_ID"),
		},
	}
}
