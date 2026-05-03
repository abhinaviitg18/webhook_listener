package app

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/config"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/integrations"
	"agenthook.store/internal/observability"
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

	pine := integrations.NewPineconeClient(cfg.PineconeEnabled, cfg.PineconeAPIKey, cfg.PineconeIndexURL, cfg.PineconeNamespace)
	llm := buildFallbackLLMClient(nil, cfg)
	tracer := observability.NewLangfuseClient(observability.Config{
		Enabled:   cfg.LangfuseEnabled,
		Host:      cfg.LangfuseHost,
		PublicKey: cfg.LangfusePublicKey,
		SecretKey: cfg.LangfuseSecretKey,
	})
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
	actions.Store = st

	processor := &service.Processor{
		Store: st, Pinecone: pine, LLM: llm, Executor: actions, Resolver: resolver, Transformer: transformer, DeterministicOnly: deterministicOnly,
		LLMCompaction: service.LLMCompactionConfig{
			Enabled:         cfg.LLMCompactionEnabled,
			ThresholdBytes:  cfg.LLMCompactionThresholdBytes,
			MaxStringBytes:  cfg.LLMCompactionMaxStringBytes,
			MaxArrayItems:   cfg.LLMCompactionMaxArrayItems,
			MaxObjectFields: cfg.LLMCompactionMaxObjectFields,
		},
		Tracer: tracer,
		BYOKResolver: func(ctx context.Context, accountID string) domain.LLMClient {
			byokCfgs, err := st.ListBYOKConfigs(ctx, accountID)
			if err != nil {
				return nil
			}
			return buildFallbackLLMClient(byokCfgs, cfg)
		},
	}

	handler := &httpapi.Handler{
		Store:                st,
		Processor:            processor,
		VerifyHTCSignature:   cfg.VerifyHTCSignature,
		AppPlan:              cfg.AppPlan,
		AppDeploymentMode:    cfg.AppDeploymentMode,
		MailDomain:           cfg.MailDomain,
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

func buildFallbackLLMClient(byokCfgs []domain.BYOKProviderConfig, cfg config.Config) domain.LLMClient {
	clients := make([]domain.LLMClient, 0, len(byokCfgs)+4)
	seen := map[string]struct{}{}
	appendClient := func(provider, key, baseURL, model string) {
		normalizedProvider := strings.TrimSpace(strings.ToLower(provider))
		trimmedKey := strings.TrimSpace(key)
		trimmedBaseURL := strings.TrimSpace(baseURL)
		trimmedModel := strings.TrimSpace(model)
		if normalizedProvider == "" || trimmedKey == "" || trimmedBaseURL == "" || trimmedModel == "" {
			return
		}
		dedupeKey := normalizedProvider + "|" + trimmedBaseURL + "|" + trimmedModel + "|" + trimmedKey
		if _, ok := seen[dedupeKey]; ok {
			return
		}
		seen[dedupeKey] = struct{}{}
		clients = append(clients, integrations.NewLLMClient(normalizedProvider, trimmedKey, trimmedBaseURL, trimmedModel))
	}

	sortedBYOK := append([]domain.BYOKProviderConfig(nil), byokCfgs...)
	slices.SortStableFunc(sortedBYOK, func(a, b domain.BYOKProviderConfig) int {
		if a.IsDefault && !b.IsDefault {
			return -1
		}
		if !a.IsDefault && b.IsDefault {
			return 1
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return strings.Compare(a.Provider, b.Provider)
	})
	for _, byokCfg := range sortedBYOK {
		appendClient(byokCfg.Provider, byokCfg.APIKey, byokCfg.BaseURL, byokCfg.Model)
	}

	appendClient(cfg.LLMProvider, cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel)
	appendClient("groq", cfg.GroqAPIKey, cfg.GroqBaseURL, cfg.GroqModel)
	appendClient("cerebras", cfg.CerebrasAPIKey, cfg.CerebrasBaseURL, cfg.CerebrasModel)
	appendClient("openrouter", cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.OpenRouterModel)
	return integrations.NewFallbackLLMClient(clients...)
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
