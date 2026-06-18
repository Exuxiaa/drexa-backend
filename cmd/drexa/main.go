package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"drexa/internal/auth"
	"drexa/internal/config"
	"drexa/internal/infrastructure/cache"
	"drexa/internal/infrastructure/database"
	firebaseInfra "drexa/internal/infrastructure/firebase"
	"drexa/internal/wallet"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DB)
	if err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(
		&auth.User{}, &auth.KycProfile{}, &auth.RefreshToken{}, &auth.PasswordResetToken{},
		&wallet.Wallet{}, &wallet.Transaction{}, &wallet.DepositRequest{}, &wallet.WithdrawalRequest{}, &wallet.CryptoAddress{},
	); err != nil {
		log.Fatalf("database migration failed: %v", err)
	}

	rdb, err := cache.NewRedis(cfg.Redis)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}

	ctx := context.Background()

	fbClient, err := firebaseInfra.New(ctx, cfg.Firebase)
	if err != nil {
		log.Printf("warning: firebase not initialized (%v) — set FIREBASE_CREDENTIALS_JSON to enable", err)
	}

	srv := NewServer(cfg, db, rdb, fbClient)

	if err := srv.Start(ctx, os.Stdout, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
