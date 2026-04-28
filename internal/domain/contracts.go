package domain

import "context"

type Store interface {
	CreateAccount(ctx context.Context, email string) (Account, string, error)
	GetAccountBySlug(ctx context.Context, slug string) (Account, error)
	GetAccountByToken(ctx context.Context, token string) (Account, error)
	GetAccount(ctx context.Context, id string) (Account, error)

	CreateWebhookType(ctx context.Context, accountID, typeKey, plainTextAction string, useLLMFallback bool) (WebhookType, error)
	ListWebhookTypes(ctx context.Context, accountID string) ([]WebhookType, error)
	GetWebhookTypeByAccountAndKey(ctx context.Context, accountID, typeKey string) (WebhookType, error)
	DeleteWebhookType(ctx context.Context, accountID, typeID string) error

	CreateSecret(ctx context.Context, accountID, typeID string) (WebhookSecret, string, error)
	ListSecrets(ctx context.Context, accountID, typeID string) ([]WebhookSecret, error)
	DeleteSecret(ctx context.Context, accountID, secretID string) error
	ValidateSecret(ctx context.Context, accountID, typeID, secret string) (WebhookSecret, error)
	ResolveSecretAnyType(ctx context.Context, accountID, secret string) (WebhookSecret, error)
	GetWebhookTypeByID(ctx context.Context, typeID string) (WebhookType, error)

	CreateForwardTarget(ctx context.Context, accountID, targetType, configJSON string) (ForwardTarget, error)
	ListForwardTargets(ctx context.Context, accountID string) ([]ForwardTarget, error)

	CreateEvent(ctx context.Context, event WebhookEvent) (WebhookEvent, error)
	UpdateEventStatus(ctx context.Context, eventID, status, action string) error
	ListEvents(ctx context.Context, accountID string, limit int) ([]WebhookEvent, error)
	GetEvent(ctx context.Context, accountID, eventID string) (WebhookEvent, error)
	FindEventBySourceEventID(ctx context.Context, accountID, sourceEventID string) (WebhookEvent, error)
	ListEventsByTag(ctx context.Context, accountID, tag string, limit int) ([]WebhookEvent, error)
	UpdateEventTags(ctx context.Context, eventID, tagsJSON string) error

	CreateTypeSignature(ctx context.Context, sig WebhookTypeSignature) (WebhookTypeSignature, error)
	ListTypeSignatures(ctx context.Context, accountID string) ([]WebhookTypeSignature, error)
	GetLatestCandidateSignature(ctx context.Context, accountID, typeKey string) (WebhookTypeSignature, error)
	SetTypeSignatureEnabled(ctx context.Context, signatureID string, enabled bool, source string) error
	CreateTransform(ctx context.Context, tr WebhookTransform) (WebhookTransform, error)
	ListTransforms(ctx context.Context, accountID, typeKey string) ([]WebhookTransform, error)
	GetActiveTransform(ctx context.Context, accountID, typeKey string) (WebhookTransform, error)
	GetLatestTransformByStatus(ctx context.Context, accountID, typeKey, status string) (WebhookTransform, error)
	SetTransformStatus(ctx context.Context, transformID, status string) error
	LogTransformRun(ctx context.Context, run TransformRun) (TransformRun, error)
	UpsertMasterPromptPolicy(ctx context.Context, accountID, promptText, updatedBy string) (MasterPromptPolicy, error)
	GetMasterPromptPolicy(ctx context.Context, accountID string) (MasterPromptPolicy, error)
	CreateWebhookSkill(ctx context.Context, skill WebhookSkill) (WebhookSkill, error)
	ListWebhookSkills(ctx context.Context, accountID, typeKey string) ([]WebhookSkill, error)
	UpsertAutoPromoteState(ctx context.Context, state AutoPromoteState) (AutoPromoteState, error)
	GetAutoPromoteState(ctx context.Context, accountID, typeKey string) (AutoPromoteState, error)

	UpsertBYOKConfig(ctx context.Context, cfg BYOKProviderConfig) (BYOKProviderConfig, error)
	GetBYOKConfig(ctx context.Context, accountID, provider string) (BYOKProviderConfig, error)
	GetDefaultBYOKConfig(ctx context.Context, accountID string) (BYOKProviderConfig, error)
	ListBYOKConfigs(ctx context.Context, accountID string) ([]BYOKProviderConfig, error)
}

type PineconeClient interface {
	Query(ctx context.Context, accountID, query string, topK int) ([]PineconeMemory, error)
	UpsertOrUpdate(ctx context.Context, accountID, typeKey, eventID, canonicalPayload string, prior []PineconeMemory) error
}

type LLMClient interface {
	SuggestAction(ctx context.Context, typeKey, payload string, memories []PineconeMemory, available []string) (ProcessDecision, error)
}

type TypeClassifier interface {
	ClassifyType(ctx context.Context, payload string, headers map[string]string, summary map[string]interface{}) (TypeResolution, error)
}

type ActionExecutor interface {
	Execute(ctx context.Context, action ProcessDecision, account Account, event WebhookEvent, targets []ForwardTarget) error
	AvailableActions() []string
}
