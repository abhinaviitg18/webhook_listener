package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"agenthook.store/internal/app"
	"agenthook.store/internal/config"
)

func main() {
	if err := config.LoadEnvFiles("local.env", ".env"); err != nil {
		log.Fatalf("env file load failed: %v", err)
	}
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}

	router, err := app.BuildHTTPHandler(context.Background(), cfg)
	if err != nil {
		log.Fatalf("build http handler failed: %v", err)
	}

	if app.UsingInMemoryStore(cfg) {
		log.Println("using in-memory store")
	} else {
		log.Println("using TiDB/MySQL store")
	}

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("agenthook server listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
