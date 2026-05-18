package main

import (
	"context"
	"drexa/internal/config"
	"drexa/internal/infrastructure/database"
	"fmt"
	"log"
	"os"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DB.DSN)
	if err != nil {
		log.Println(err)
	}

	srv := NewServer(cfg, db)

	ctx := context.Background()
	if err := srv.Start(ctx, os.Stdout, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
