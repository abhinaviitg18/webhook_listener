package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port              string
	AppPlan           string
	AppDeploymentMode string

	ScaleKitBaseURL      string
	ScaleKitAPIKey       string
	ScaleKitClientID     string
	ScaleKitClientSecret string
	ScaleKitRedirectURI  string
	AppSessionSecret     string
	PublicBaseURL        string

	TiDBDSN string

	PineconeAPIKey    string
	PineconeIndexURL  string
	PineconeNamespace string

	LLMProvider                  string
	LLMAPIKey                    string
	LLMBaseURL                   string
	LLMModel                     string
	LLMCompactionEnabled         bool
	LLMCompactionThresholdBytes  int
	LLMCompactionMaxStringBytes  int
	LLMCompactionMaxArrayItems   int
	LLMCompactionMaxObjectFields int

	GroqAPIKey        string
	GroqBaseURL       string
	GroqModel         string
	CerebrasAPIKey    string
	CerebrasBaseURL   string
	CerebrasModel     string
	OpenRouterAPIKey  string
	OpenRouterBaseURL string
	OpenRouterModel   string

	TelegramBotToken string

	LangfuseEnabled   bool
	LangfuseHost      string
	LangfusePublicKey string
	LangfuseSecretKey string

	MailDBDSN                 string
	MailDomain                string
	MailAWSRegion             string
	MailInboundBucket         string
	MailOutboundProvider      string
	MailAgentHookBaseURL      string
	MailAgentHookOriginSecret string
	MailInternalSharedSecret  string
	MailResendAPIKey          string
	MailResendBaseURL         string
	MailPostmarkServerToken   string
	MailPostmarkBaseURL       string
	MailSMTPHost              string
	MailSMTPPort              int
	MailSMTPUsername          string
	MailSMTPPassword          string
	MailSMTPUseTLS            bool
	MailZeptoMailAPIKey       string
	MailZeptoMailBaseURL      string

	UseInMemoryStore          bool
	VerifyHTCSignature        bool
	DeterministicOnlyTypeKeys []string

	AutoPromoteEnabled           bool
	AutoPromoteMinConfidence     float64
	AutoPromoteValidatedToShadow int
	AutoPromoteShadowToActive    int
	AutoPromoteMinSuccessRate    float64
}

func Load() Config {
	storeDSN := getenv("COMMERCE_MYSQL_DSN", "")
	if storeDSN == "" {
		storeDSN = getenv("TIDB_DSN", "")
	}
	defaultInMemoryStore := strings.TrimSpace(storeDSN) == ""

	llmKey := getenv("LLM_API_KEY", "")
	if llmKey == "" {
		llmKey = getenv("OPENROUTER_API_KEY", "")
	}
	pineconeHost := getenv("PINECONE_INDEX_URL", "")

	return Config{
		Port:              getenv("PORT", "8080"),
		AppPlan:           normalizePlan(getenv("APP_PLAN", "basic")),
		AppDeploymentMode: normalizeDeploymentModeEnv(getenv("APP_DEPLOYMENT_MODE", "")),

		ScaleKitBaseURL:      getenv("SCALEKIT_BASE_URL", ""),
		ScaleKitAPIKey:       getenv("SCALEKIT_API_KEY", ""),
		ScaleKitClientID:     getenv("SCALEKIT_CLIENT_ID", ""),
		ScaleKitClientSecret: getenv("SCALEKIT_CLIENT_SECRET", ""),
		ScaleKitRedirectURI:  getenv("SCALEKIT_REDIRECT_URI", ""),
		AppSessionSecret:     getenv("APP_SESSION_SECRET", getenv("SCALEKIT_CLIENT_SECRET", "")),
		PublicBaseURL:        getenv("PUBLIC_BASE_URL", "https://app.agenthook.store"),

		TiDBDSN: storeDSN,

		PineconeAPIKey:    getenv("PINECONE_API_KEY", ""),
		PineconeIndexURL:  pineconeHost,
		PineconeNamespace: getenv("PINECONE_NAMESPACE", "default"),

		LLMProvider:                  getenv("LLM_PROVIDER", "openrouter"),
		LLMAPIKey:                    llmKey,
		LLMBaseURL:                   getenv("LLM_BASE_URL", "https://openrouter.ai/api/v1"),
		LLMModel:                     getenv("LLM_MODEL", "openai/gpt-4o-mini"),
		LLMCompactionEnabled:         getbool("LLM_COMPACTION_ENABLED", true),
		LLMCompactionThresholdBytes:  getint("LLM_COMPACTION_THRESHOLD_BYTES", 6000),
		LLMCompactionMaxStringBytes:  getint("LLM_COMPACTION_MAX_STRING_BYTES", 400),
		LLMCompactionMaxArrayItems:   getint("LLM_COMPACTION_MAX_ARRAY_ITEMS", 8),
		LLMCompactionMaxObjectFields: getint("LLM_COMPACTION_MAX_OBJECT_FIELDS", 20),

		GroqAPIKey:        getenv("GROQ_API_KEY", ""),
		GroqBaseURL:       getenv("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
		GroqModel:         getenv("GROQ_MODEL", ""),
		CerebrasAPIKey:    getenv("CEREBRAS_API_KEY", ""),
		CerebrasBaseURL:   getenv("CEREBRAS_BASE_URL", "https://api.cerebras.ai/v1"),
		CerebrasModel:     getenv("CEREBRAS_MODEL", "llama-3.3-70b"),
		OpenRouterAPIKey:  getenv("OPENROUTER_API_KEY", llmKey),
		OpenRouterBaseURL: getenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		OpenRouterModel:   getenv("OPENROUTER_MODEL", "openrouter/free"),

		TelegramBotToken: getenv("TELEGRAM_BOT_TOKEN", ""),

		LangfuseEnabled:   getbool("LANGFUSE_ENABLED", false),
		LangfuseHost:      getenv("LANGFUSE_HOST", "https://cloud.langfuse.com"),
		LangfusePublicKey: getenv("LANGFUSE_PUBLIC_KEY", ""),
		LangfuseSecretKey: getenv("LANGFUSE_SECRET_KEY", ""),

		MailDBDSN:                 getenv("MAIL_DB_DSN", storeDSN),
		MailDomain:                getenv("MAIL_DOMAIN", "app.agenthook.store"),
		MailAWSRegion:             getenv("MAIL_AWS_REGION", getenv("AWS_REGION", "us-east-1")),
		MailInboundBucket:         getenv("MAIL_INBOUND_BUCKET", ""),
		MailOutboundProvider:      normalizeMailOutboundProvider(getenv("MAIL_OUTBOUND_PROVIDER", "ses")),
		MailAgentHookBaseURL:      getenv("MAIL_AGENTHOOK_BASE_URL", getenv("PUBLIC_BASE_URL", "https://app.agenthook.store")),
		MailAgentHookOriginSecret: getenv("MAIL_AGENTHOOK_ORIGIN_SECRET", getenv("LAMBDA_ORIGIN_SHARED_SECRET", "")),
		MailInternalSharedSecret:  getenv("MAIL_INTERNAL_SHARED_SECRET", ""),
		MailResendAPIKey:          getenv("MAIL_RESEND_API_KEY", ""),
		MailResendBaseURL:         getenv("MAIL_RESEND_BASE_URL", "https://api.resend.com"),
		MailPostmarkServerToken:   getenv("MAIL_POSTMARK_SERVER_TOKEN", ""),
		MailPostmarkBaseURL:       getenv("MAIL_POSTMARK_BASE_URL", "https://api.postmarkapp.com"),
		MailSMTPHost:              getenv("MAIL_SMTP_HOST", ""),
		MailSMTPPort:              getint("MAIL_SMTP_PORT", 587),
		MailSMTPUsername:          getenv("MAIL_SMTP_USERNAME", ""),
		MailSMTPPassword:          getenv("MAIL_SMTP_PASSWORD", ""),
		MailSMTPUseTLS:            getbool("MAIL_SMTP_USE_TLS", true),
		MailZeptoMailAPIKey:       getenv("MAIL_ZEPTOMAIL_API_KEY", ""),
		MailZeptoMailBaseURL:      getenv("MAIL_ZEPTOMAIL_BASE_URL", "https://api.zeptomail.com/v1.1"),

		UseInMemoryStore:             getbool("USE_IN_MEMORY_STORE", defaultInMemoryStore),
		VerifyHTCSignature:           getbool("VERIFY_HTC_SIGNATURE", false),
		DeterministicOnlyTypeKeys:    splitCSV(getenv("DETERMINISTIC_ONLY_TYPE_KEYS", "ai-recruiter-inbox-message")),
		AutoPromoteEnabled:           getbool("AUTOPROMOTE_ENABLED", true),
		AutoPromoteMinConfidence:     getfloat("AUTOPROMOTE_MIN_CONFIDENCE", 0.88),
		AutoPromoteValidatedToShadow: getint("AUTOPROMOTE_VALIDATED_TO_SHADOW", 2),
		AutoPromoteShadowToActive:    getint("AUTOPROMOTE_SHADOW_TO_ACTIVE", 3),
		AutoPromoteMinSuccessRate:    getfloat("AUTOPROMOTE_MIN_SUCCESS_RATE", 0.9),
	}
}

func normalizePlan(in string) string {
	switch strings.TrimSpace(strings.ToLower(in)) {
	case "enterprise":
		return "enterprise"
	default:
		return "basic"
	}
}

func normalizeDeploymentModeValue(in string) string {
	switch strings.TrimSpace(strings.ToLower(in)) {
	case "enterprise", "single_tenant":
		return "single_tenant"
	default:
		return "multitenant"
	}
}

func normalizeDeploymentModeEnv(in string) string {
	trimmed := strings.TrimSpace(strings.ToLower(in))
	if trimmed == "" {
		return ""
	}
	return normalizeDeploymentModeValue(trimmed)
}

func normalizeMailOutboundProvider(in string) string {
	switch strings.TrimSpace(strings.ToLower(in)) {
	case "resend":
		return "resend"
	case "postmark":
		return "postmark"
	case "smtp":
		return "smtp"
	case "zeptomail":
		return "zeptomail"
	default:
		return "ses"
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Port) == "" {
		return fmt.Errorf("PORT is required")
	}
	if !c.UseInMemoryStore && strings.TrimSpace(c.TiDBDSN) == "" {
		return fmt.Errorf("TIDB_DSN is required when USE_IN_MEMORY_STORE=false")
	}
	return nil
}

func getenv(k, fallback string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	return v
}

func getbool(k string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getfloat(k string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getint(k string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func splitCSV(in string) []string {
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(strings.ToLower(p))
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}
