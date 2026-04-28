package app

import (
	"context"
	"net/http"
	"strings"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/config"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/integrations"
	"agenthook.store/internal/service"
	"agenthook.store/internal/store"
	httpapi "agenthook.store/internal/transport/http"
)

func BuildHTTPHandler(ctx context.Context, cfg config.Config) (http.Handler, error) {
	var st domain.Store

	if cfg.UseInMemoryStore {
		st = store.NewMemoryStore()
	} else {
		mysqlStore, err := store.NewMySQLStore(cfg.TiDBDSN)
		if err != nil {
			return nil, err
		}
		st = mysqlStore
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

	processor := &service.Processor{
		Store: st, Pinecone: pine, LLM: llm, Executor: actions, Resolver: resolver, Transformer: transformer, DeterministicOnly: deterministicOnly,
		LLMCompaction: service.LLMCompactionConfig{
			Enabled:         cfg.LLMCompactionEnabled,
			ThresholdBytes:  cfg.LLMCompactionThresholdBytes,
			MaxStringBytes:  cfg.LLMCompactionMaxStringBytes,
			MaxArrayItems:   cfg.LLMCompactionMaxArrayItems,
			MaxObjectFields: cfg.LLMCompactionMaxObjectFields,
		},
		BYOKResolver: func(ctx context.Context, accountID string) domain.LLMClient {
			byokCfg, err := st.GetDefaultBYOKConfig(ctx, accountID)
			if err != nil {
				return nil
			}
			return integrations.NewLLMClient(byokCfg.Provider, byokCfg.APIKey, byokCfg.BaseURL, byokCfg.Model)
		},
	}

	handler := &httpapi.Handler{
		Store:                st,
		Processor:            processor,
		VerifyHTCSignature:   cfg.VerifyHTCSignature,
		ScaleKitBaseURL:      cfg.ScaleKitBaseURL,
		ScaleKitClientID:     cfg.ScaleKitClientID,
		ScaleKitClientSecret: cfg.ScaleKitClientSecret,
		ScaleKitRedirectURI:  cfg.ScaleKitRedirectURI,
		AppSessionSecret:     cfg.AppSessionSecret,
		PublicBaseURL:        cfg.PublicBaseURL,
	}
	verifier := auth.TokenVerifier{
		Store:           st,
		ScaleKitBaseURL: cfg.ScaleKitBaseURL,
		ScaleKitAPIKey:  cfg.ScaleKitAPIKey,
	}

	return httpapi.NewRouter(handler, verifier), nil
}

func UsingInMemoryStore(cfg config.Config) bool {
	return cfg.UseInMemoryStore
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
