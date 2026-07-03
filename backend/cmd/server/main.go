package main

import (
	"log"

	"marketingflow/internal/config"
	"marketingflow/internal/database"
	"marketingflow/internal/handler"
	"marketingflow/internal/repository"
	"marketingflow/internal/seed"
)

func main() {
	cfg := config.Load()

	for _, w := range cfg.SecurityWarnings() {
		log.Printf("[SECURITY WARNING] %s", w)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}

	// Seed the default accounts on first run.
	if err := seed.Accounts(repository.NewUserRepository(db), cfg); err != nil {
		log.Fatalf("seeding failed: %v", err)
	}

	router := handler.NewRouter(db, cfg)

	addr := ":" + cfg.AppPort
	log.Printf("Marketing Flow API listening on %s (env=%s)", addr, cfg.AppEnv)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
