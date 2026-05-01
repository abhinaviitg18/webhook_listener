package domain

import "context"

type Store interface {
	CreateAccount(ctx context.Context, email string) (Account, string, error)
	GetAccountBySlug(ctx context.Context, slug string) (Account, error)
	GetAccountByPublicAlias(ctx context.Context, alias string) (Account, error)
	GetAccountByToken(ctx context.Context, token string) (Account, error)
	GetAccount(ctx context.Context, id string) (Account, error)
	UpdateAccountPublicAlias(ctx context.Context, accountID, publicAlias string) (Account, error)

	ListAccountTokens(ctx context.Context, accountID string) ([]AccountToken, error)
	RevokeAccountToken(ctx context.Context, accountID, tokenID string) error

	CreateWebhookType(ctx context.Context, accountID, typeKey, plainTextAction string, useLLMFallback bool) (WebhookType, error)
	ListWebhookTypes(ctx context.Context, accountID string) ([]WebhookType, error)
	GetWebhookTypeByAccountAndKey(ctx context.Context, accountID, typeKey string) (WebhookType, error)
	DeleteWebhookType(ctx context.Context, accountID, typeID string) error

	CreateSecret(ctx context.Context, accountID, typeID string) (WebhookSecret, string, error)
	CreateSecretWithValue(ctx context.Context, accountID, typeID, secretValue string) (WebhookSecret, error)
	ListSecrets(ctx context.Context, accountID, typeID string) ([]WebhookSecret, error)
	DeleteSecret(ctx context.Context, accountID, secretID string) error
	ValidateSecret(ctx context.Context, accountID, typeID, secret string) (WebhookSecret, error)
	ResolveSecretAnyType(ctx context.Context, accountID, secret string) (WebhookSecret, error)
	ListWebhookIdentities(ctx context.Context, accountID string) ([]WebhookIdentity, error)
	GetWebhookIdentityByLocalPart(ctx context.Context, localPart string) (WebhookIdentity, error)
	UpdateWebhookIdentityStatus(ctx context.Context, accountID, identityID, status string) (WebhookIdentity, error)
	GetWebhookTypeByID(ctx context.Context, typeID string) (WebhookType, error)

	CreateForwardTarget(ctx context.Context, accountID, targetType, configJSON string) (ForwardTarget, error)
	ListForwardTargets(ctx context.Context, accountID string) ([]ForwardTarget, error)
	UpdateForwardTarget(ctx context.Context, target ForwardTarget) (ForwardTarget, error)
	DeleteForwardTarget(ctx context.Context, accountID, targetID string) error
	CreateIntegrationSecret(ctx context.Context, secret IntegrationSecret) (IntegrationSecret, error)
	ListIntegrationSecrets(ctx context.Context, accountID string) ([]IntegrationSecret, error)
	UpdateIntegrationSecret(ctx context.Context, secret IntegrationSecret) (IntegrationSecret, error)
	DeleteIntegrationSecret(ctx context.Context, accountID, secretID string) error
	ResolveIntegrationSecretValue(ctx context.Context, accountID, secretKey string) (string, error)

	CreateEvent(ctx context.Context, event WebhookEvent) (WebhookEvent, error)
	UpdateEventStatus(ctx context.Context, eventID, status, action string) error
	UpdateEventProcessedText(ctx context.Context, eventID, processedText string) error
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
	ListWebhookSkillsIncludingDisabled(ctx context.Context, accountID, typeKey string) ([]WebhookSkill, error)
	UpdateWebhookSkill(ctx context.Context, skill WebhookSkill) (WebhookSkill, error)
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
