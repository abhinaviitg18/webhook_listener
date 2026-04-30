package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/config"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/mail"
	mailhttp "agenthook.store/internal/mail/httpapi"
	"agenthook.store/internal/store"
)

func main() {
	if err := config.LoadEnvFiles("local.env", ".env"); err != nil {
		log.Fatalf("env file load failed: %v", err)
	}
	cfg := config.Load()

	accountStore, err := buildAccountStore(cfg)
	if err != nil {
		log.Fatalf("build account store failed: %v", err)
	}
	mailService, err := mail.NewServiceFromConfig(context.Background(), cfg, accountStore)
	if err != nil {
		log.Fatalf("build mail service failed: %v", err)
	}

	handler := mailhttp.NewRouter(&mailhttp.Handler{
		Service:      mailService,
		AccountStore: accountStore,
	}, auth.TokenVerifier{Store: accountStore})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("agenthook mail api listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func buildAccountStore(cfg config.Config) (domain.Store, error) {
	if cfg.UseInMemoryStore {
		return store.NewMemoryStore(), nil
	}
	return store.NewMySQLStore(cfg.TiDBDSN)
}
