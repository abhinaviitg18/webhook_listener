package domain

import "time"

type Account struct {
	ID         string    `json:"id"`
	Slug       string    `json:"slug"`
	OwnerEmail string    `json:"owner_email"`
	TokenHash  string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}

type WebhookType struct {
	ID              string    `json:"id"`
	AccountID       string    `json:"account_id"`
	TypeKey         string    `json:"type_key"`
	PlainTextAction string    `json:"plain_text_action"`
	UseLLMFallback  bool      `json:"use_llm_fallback"`
	CreatedAt       time.Time `json:"created_at"`
}

type WebhookSecret struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	TypeID      string    `json:"type_id"`
	ValueHash   string    `json:"-"`
	SecretValue string    `json:"-"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type ForwardTarget struct {
	ID         string    `json:"id"`
	AccountID  string    `json:"account_id"`
	TargetType string    `json:"target_type"`
	ConfigJSON string    `json:"config_json"`
	CreatedAt  time.Time `json:"created_at"`
}

type WebhookEvent struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"account_id"`
	TypeID         string    `json:"type_id"`
	SecretID       string    `json:"secret_id"`
	RequestID      string    `json:"request_id"`
	SourceEventID  string    `json:"source_event_id,omitempty"`
	TypeKey        string    `json:"type_key"`
	RawPayloadJSON string    `json:"raw_payload_json,omitempty"`
	PayloadJSON    string    `json:"payload_json"`
	ProcessedText  string    `json:"processed_text,omitempty"`
	ActionSelected string    `json:"action_selected"`
	TagsJSON       string    `json:"tags_json,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type WebhookTypeSignature struct {
	ID                  string    `json:"id"`
	AccountID           string    `json:"account_id"`
	TypeKey             string    `json:"type_key"`
	Version             int       `json:"version"`
	RequiredKeysJSON    string    `json:"required_keys_json"`
	ShapeHintsJSON      string    `json:"shape_hints_json"`
	HeaderHintsJSON     string    `json:"header_hints_json"`
	ConfidenceThreshold float64   `json:"confidence_threshold"`
	Enabled             bool      `json:"enabled"`
	Source              string    `json:"source"`
	CreatedAt           time.Time `json:"created_at"`
}

type WebhookTransform struct {
	ID                     string    `json:"id"`
	AccountID              string    `json:"account_id"`
	TypeKey                string    `json:"type_key"`
	Version                int       `json:"version"`
	Engine                 string    `json:"engine"`
	WASMBlobRef            string    `json:"wasm_blob_ref"`
	DSLText                string    `json:"dsl_text"`
	DeterministicTestsJSON string    `json:"deterministic_tests_json"`
	Status                 string    `json:"status"`
	CreatedAt              time.Time `json:"created_at"`
}

type TransformRun struct {
	ID               string    `json:"id"`
	EventID          string    `json:"event_id"`
	AccountID        string    `json:"account_id"`
	TypeKey          string    `json:"type_key"`
	TransformVersion int       `json:"transform_version"`
	DurationMS       int64     `json:"duration_ms"`
	ResultHash       string    `json:"result_hash"`
	ErrorText        string    `json:"error_text"`
	CreatedAt        time.Time `json:"created_at"`
}

type TypeResolution struct {
	TypeKey                 string                 `json:"type_key"`
	Confidence              float64                `json:"confidence"`
	Source                  string                 `json:"source"`
	Reason                  string                 `json:"reason"`
	ExtractFields           map[string]string      `json:"extract_fields"`
	TransformTemplateEngine string                 `json:"transform_template_engine"`
	TransformTemplate       map[string]interface{} `json:"transform_template"`
	ManualReview            bool                   `json:"manual_review"`
}

type TransformResult struct {
	CanonicalPayload string `json:"canonical_payload"`
	Engine           string `json:"engine"`
	Version          int    `json:"version"`
}

type ProcessDecision struct {
	ActionName    string                 `json:"action_name"`
	Reason        string                 `json:"reason"`
	Params        map[string]interface{} `json:"params"`
	ProcessedText string                 `json:"processed_text,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
}

type PineconeMemory struct {
	ID      string                 `json:"id"`
	Score   float64                `json:"score"`
	Summary string                 `json:"summary"`
	Meta    map[string]interface{} `json:"meta"`
}

type MasterPromptPolicy struct {
	AccountID  string    `json:"account_id"`
	PromptText string    `json:"prompt_text"`
	UpdatedBy  string    `json:"updated_by"`
	UpdatedAt  time.Time `json:"updated_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type WebhookSkill struct {
	ID              string    `json:"id"`
	AccountID       string    `json:"account_id"`
	TypeKey         string    `json:"type_key"`
	SkillKey        string    `json:"skill_key"`
	SkillPrompt     string    `json:"skill_prompt"`
	MatchContains   string    `json:"match_contains"`
	ForcedAction    string    `json:"forced_action"`
	MemoryWriteMode string    `json:"memory_write_mode"`
	Priority        int       `json:"priority"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
}

type AutoPromoteState struct {
	AccountID      string    `json:"account_id"`
	TypeKey        string    `json:"type_key"`
	Status         string    `json:"status"`
	ValidatedCount int       `json:"validated_count"`
	ShadowTotal    int       `json:"shadow_total"`
	ShadowSuccess  int       `json:"shadow_success"`
	LastConfidence float64   `json:"last_confidence"`
	LastReason     string    `json:"last_reason"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type BYOKProviderConfig struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Provider  string    `json:"provider"`
	APIKey    string    `json:"-"`
	BaseURL   string    `json:"base_url"`
	Model     string    `json:"model"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}
