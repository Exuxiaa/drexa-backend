package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"drexa/internal/platform/migrate"
	"drexa/internal/platform/postgres"
	"drexa/pkg/config"
	"drexa/pkg/logger"
)

func main() {
	cfg := config.Load()
	logger.Init(cfg.App.Env)

	db, err := postgres.Connect(cfg.DB)
	if err != nil {
		log.Fatal().Err(err).Msg("database: connect failed")
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal().Err(err).Msg("database: get sql.DB failed")
	}

	if err := migrate.Up(sqlDB, "migrations"); err != nil {
		log.Fatal().Err(err).Msg("migrations failed")
	}
	log.Info().Msg("migrations: up to date")

	srv := NewServer(cfg, db)

	if err := srv.Start(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
