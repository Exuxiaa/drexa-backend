package config

import (
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig
	DB       DBConfig
	JWT      JWTConfig
	Twilio   TwilioConfig
	SendGrid SendGridConfig
	Tatum    TatumConfig
	Google   GoogleConfig
  Stripe   StripeConfig
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

func Load() *Config {
	_ = godotenv.Load() // optional; env vars take precedence
	viper.AutomaticEnv()

	viper.SetDefault("APP_PORT", ":8080")
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("DB_MAX_IDLE_CONNS", 10)
	viper.SetDefault("DB_MAX_OPEN_CONNS", 100)
	viper.SetDefault("DB_CONN_MAX_LIFETIME", "1h")
	viper.SetDefault("JWT_ACCESS_EXPIRATION", "15m")
	viper.SetDefault("JWT_REFRESH_EXPIRATION", "168h")
	viper.SetDefault("SENDGRID_FROM_NAME", "Drexa")
	viper.SetDefault("APP_URL", "http://localhost:3000")

	return &Config{
		App: AppConfig{
			Port:           viper.GetString("APP_PORT"),
			Env:            viper.GetString("APP_ENV"),
			AllowedOrigins: []string{"http://localhost:3000", "http://localhost:3001"},
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
		Google: GoogleConfig{
			ClientID: viper.GetString("GOOGLE_CLIENT_ID"),
		},
	}
}
