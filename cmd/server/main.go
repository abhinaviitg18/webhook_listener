package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"hookweb.club/internal/auth"
	"hookweb.club/internal/config"
	"hookweb.club/internal/domain"
	"hookweb.club/internal/integrations"
	"hookweb.club/internal/service"
	"hookweb.club/internal/store"
	httpapi "hookweb.club/internal/transport/http"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}

	var st domain.Store

	if cfg.UseInMemoryStore {
		st = store.NewMemoryStore()
		log.Println("using in-memory store")
	} else {
		mysqlStore, err := store.NewMySQLStore(cfg.TiDBDSN)
		if err != nil {
			log.Fatalf("mysql init failed: %v", err)
		}
		if err := mysqlStore.Ping(context.Background()); err != nil {
			log.Fatalf("mysql ping failed: %v", err)
		}
		st = mysqlStore
		log.Println("using TiDB/MySQL store")
	}

	pine := integrations.NewPineconeClient(cfg.PineconeAPIKey, cfg.PineconeIndexURL, cfg.PineconeNamespace)
	llm := integrations.NewLLMClient(cfg.LLMProvider, cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel)
	groqClassifier := integrations.NewProviderTypeClassifier("groq", cfg.GroqBaseURL, cfg.GroqAPIKey, cfg.GroqModel)
	cerebrasClassifier := integrations.NewProviderTypeClassifier("cerebras", cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, cfg.CerebrasModel)
	deterministicOnly := toSet(cfg.DeterministicOnlyTypeKeys)
	autoPromoter := service.NewAutoPromoter(st, service.AutoPromoteConfig{
		Enabled:           cfg.AutoPromoteEnabled,
		MinConfidence:     cfg.AutoPromoteMinConfidence,
		ValidatedToShadow: cfg.AutoPromoteValidatedToShadow,
		ShadowToActive:    cfg.AutoPromoteShadowToActive,
		MinSuccessRate:    cfg.AutoPromoteMinSuccessRate,
		DeterministicOnly: deterministicOnly,
	})
	resolver := service.NewTypeResolver(st, groqClassifier, cerebrasClassifier)
	resolver.AutoPromoter = autoPromoter
	resolver.DeterministicOnly = deterministicOnly
	transformer := service.NewTransformService(st)
	tg := integrations.NewTelegramClient(cfg.TelegramBotToken)
	actions := service.NewActionService(tg)

	processor := &service.Processor{Store: st, Pinecone: pine, LLM: llm, Executor: actions, Resolver: resolver, Transformer: transformer, DeterministicOnly: deterministicOnly}
	handler := &httpapi.Handler{
		Store:              st,
		Processor:          processor,
		VerifyHTCSignature: cfg.VerifyHTCSignature,
		ScaleKitBaseURL:    cfg.ScaleKitBaseURL,
	}
	verifier := auth.TokenVerifier{
		Store:           st,
		ScaleKitBaseURL: cfg.ScaleKitBaseURL,
		ScaleKitAPIKey:  cfg.ScaleKitAPIKey,
	}
	router := httpapi.NewRouter(handler, verifier)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("hookweb server listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func toSet(items []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		key := strings.TrimSpace(strings.ToLower(item))
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}
