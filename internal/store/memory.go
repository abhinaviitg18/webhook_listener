package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/security"
)

type MemoryStore struct {
	mu sync.RWMutex

	accountsByID    map[string]domain.Account
	accountsBySlug  map[string]string
	accountsByAlias map[string]string
	tokens          map[string]string

	typesByID         map[string]domain.WebhookType
	typesByAccountKey map[string]string

	secretsByID  map[string]domain.WebhookSecret
	secretByHash map[string]string

	targets            map[string]domain.ForwardTarget
	integrationSecrets map[string]domain.IntegrationSecret
	events             map[string]domain.WebhookEvent
	eventBySource      map[string]string

	signatures map[string]domain.WebhookTypeSignature
	transforms map[string]domain.WebhookTransform
	runs       map[string]domain.TransformRun

	policiesByAccount map[string]domain.MasterPromptPolicy
	skills            map[string]domain.WebhookSkill
	autoStates        map[string]domain.AutoPromoteState
	byokConfigs       map[string]domain.BYOKProviderConfig
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		accountsByID:       map[string]domain.Account{},
		accountsBySlug:     map[string]string{},
		accountsByAlias:    map[string]string{},
		tokens:             map[string]string{},
		typesByID:          map[string]domain.WebhookType{},
		typesByAccountKey:  map[string]string{},
		secretsByID:        map[string]domain.WebhookSecret{},
		secretByHash:       map[string]string{},
		targets:            map[string]domain.ForwardTarget{},
		integrationSecrets: map[string]domain.IntegrationSecret{},
		events:             map[string]domain.WebhookEvent{},
		eventBySource:      map[string]string{},
		signatures:         map[string]domain.WebhookTypeSignature{},
		transforms:         map[string]domain.WebhookTransform{},
		runs:               map[string]domain.TransformRun{},
		policiesByAccount:  map[string]domain.MasterPromptPolicy{},
		skills:             map[string]domain.WebhookSkill{},
		autoStates:         map[string]domain.AutoPromoteState{},
	}
}

func (s *MemoryStore) CreateAccount(_ context.Context, email string) (domain.Account, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slug := slugFromEmail(email)
	if id, ok := s.accountsBySlug[slug]; ok {
		acct := s.accountsByID[id]
		token, err := security.NewToken(24)
		if err != nil {
			return domain.Account{}, "", err
		}
		s.tokens[security.HashValue(token)] = acct.ID
		return acct, token, nil
	}
	id := uuid.NewString()
	token, err := security.NewToken(24)
	if err != nil {
		return domain.Account{}, "", err
	}
	acct := domain.Account{
		ID:          id,
		Slug:        slug,
		PublicAlias: slug,
		OwnerEmail:  email,
		TokenHash:   security.HashValue(token),
		CreatedAt:   time.Now().UTC(),
	}
	s.accountsByID[id] = acct
	s.accountsBySlug[slug] = id
	s.accountsByAlias[acct.PublicAlias] = id
	s.tokens[acct.TokenHash] = id
	return acct, token, nil
}

func (s *MemoryStore) GetAccountBySlug(_ context.Context, slug string) (domain.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.accountsBySlug[slug]
	if !ok {
		return domain.Account{}, errors.New("account not found")
	}
	return s.accountsByID[id], nil
}

func (s *MemoryStore) GetAccountByPublicAlias(_ context.Context, alias string) (domain.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.accountsByAlias[alias]
	if !ok {
		return domain.Account{}, errors.New("account not found")
	}
	return s.accountsByID[id], nil
}

func (s *MemoryStore) GetAccountByToken(_ context.Context, token string) (domain.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.tokens[security.HashValue(token)]
	if !ok {
		return domain.Account{}, errors.New("unauthorized")
	}
	return s.accountsByID[id], nil
}

func (s *MemoryStore) ListAccountTokens(_ context.Context, accountID string) ([]domain.AccountToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.AccountToken
	for tokenHash, storedAccountID := range s.tokens {
		if storedAccountID == accountID {
			out = append(out, domain.AccountToken{
				ID:        tokenHash,
				AccountID: accountID,
				CreatedAt: time.Now().UTC(),
			})
		}
	}
	return out, nil
}

func (s *MemoryStore) RevokeAccountToken(_ context.Context, accountID, tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if storedAccountID, ok := s.tokens[tokenID]; ok && storedAccountID == accountID {
		delete(s.tokens, tokenID)
		return nil
	}
	return errors.New("token not found")
}

func (s *MemoryStore) GetAccount(_ context.Context, id string) (domain.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acct, ok := s.accountsByID[id]
	if !ok {
		return domain.Account{}, errors.New("account not found")
	}
	return acct, nil
}

func (s *MemoryStore) UpdateAccountPublicAlias(_ context.Context, accountID, publicAlias string) (domain.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	acct, ok := s.accountsByID[accountID]
	if !ok {
		return domain.Account{}, errors.New("account not found")
	}
	if existingID, ok := s.accountsByAlias[publicAlias]; ok && existingID != accountID {
		return domain.Account{}, errors.New("public alias already in use")
	}
	delete(s.accountsByAlias, acct.PublicAlias)
	acct.PublicAlias = publicAlias
	s.accountsByID[accountID] = acct
	s.accountsByAlias[publicAlias] = accountID
	return acct, nil
}

func (s *MemoryStore) CreateWebhookType(_ context.Context, accountID, typeKey, plainTextAction string, useLLMFallback bool) (domain.WebhookType, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := uuid.NewString()
	obj := domain.WebhookType{
		ID:              id,
		AccountID:       accountID,
		TypeKey:         typeKey,
		PlainTextAction: plainTextAction,
		UseLLMFallback:  useLLMFallback,
		CreatedAt:       time.Now().UTC(),
	}
	s.typesByID[id] = obj
	s.typesByAccountKey[accountID+"::"+typeKey] = id
	return obj, nil
}

func (s *MemoryStore) GetWebhookTypeByID(_ context.Context, typeID string) (domain.WebhookType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.typesByID[typeID]
	if !ok {
		return domain.WebhookType{}, errors.New("type not found")
	}
	return t, nil
}

func (s *MemoryStore) ListWebhookTypes(_ context.Context, accountID string) ([]domain.WebhookType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.WebhookType
	for _, t := range s.typesByID {
		if t.AccountID == accountID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) GetWebhookTypeByAccountAndKey(_ context.Context, accountID, typeKey string) (domain.WebhookType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.typesByAccountKey[accountID+"::"+typeKey]
	if !ok {
		return domain.WebhookType{}, errors.New("type not found")
	}
	return s.typesByID[id], nil
}

func (s *MemoryStore) DeleteWebhookType(_ context.Context, accountID, typeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	wt, ok := s.typesByID[typeID]
	if !ok || wt.AccountID != accountID {
		return errors.New("type not found")
	}
	delete(s.typesByID, typeID)
	delete(s.typesByAccountKey, accountID+"::"+wt.TypeKey)
	return nil
}

func (s *MemoryStore) CreateSecret(_ context.Context, accountID, typeID string) (domain.WebhookSecret, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := security.NewToken(18)
	if err != nil {
		return domain.WebhookSecret{}, "", err
	}
	id := uuid.NewString()
	obj := domain.WebhookSecret{
		ID:          id,
		AccountID:   accountID,
		TypeID:      typeID,
		SecretValue: raw,
		Status:      "active",
		CreatedAt:   time.Now().UTC(),
	}
	s.secretsByID[id] = obj
	s.secretByHash[raw] = id
	return obj, raw, nil
}

func (s *MemoryStore) CreateSecretWithValue(_ context.Context, accountID, typeID, secretValue string) (domain.WebhookSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.secretByHash[secretValue]; ok {
		existing := s.secretsByID[existingID]
		if existing.AccountID == accountID && existing.Status == "active" {
			return domain.WebhookSecret{}, errors.New("secret already in use")
		}
	}
	id := uuid.NewString()
	obj := domain.WebhookSecret{
		ID:          id,
		AccountID:   accountID,
		TypeID:      typeID,
		SecretValue: secretValue,
		Status:      "active",
		CreatedAt:   time.Now().UTC(),
	}
	s.secretsByID[id] = obj
	s.secretByHash[secretValue] = id
	return obj, nil
}

func (s *MemoryStore) ListSecrets(_ context.Context, accountID, typeID string) ([]domain.WebhookSecret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.WebhookSecret
	for _, sec := range s.secretsByID {
		if sec.AccountID == accountID && sec.TypeID == typeID && sec.Status == "active" {
			out = append(out, sec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) DeleteSecret(_ context.Context, accountID, secretID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.secretsByID[secretID]
	if !ok || sec.AccountID != accountID {
		return errors.New("secret not found")
	}
	sec.Status = "revoked"
	s.secretsByID[secretID] = sec
	return nil
}

func (s *MemoryStore) ValidateSecret(_ context.Context, accountID, typeID, secret string) (domain.WebhookSecret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.secretByHash[secret]
	if !ok {
		return domain.WebhookSecret{}, errors.New("invalid secret")
	}
	sec := s.secretsByID[id]
	if sec.AccountID != accountID || sec.TypeID != typeID || sec.Status != "active" {
		return domain.WebhookSecret{}, errors.New("invalid secret")
	}
	return sec, nil
}

func (s *MemoryStore) ResolveSecretAnyType(_ context.Context, accountID, secret string) (domain.WebhookSecret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.secretByHash[secret]
	if !ok {
		return domain.WebhookSecret{}, errors.New("invalid secret")
	}
	sec := s.secretsByID[id]
	if sec.AccountID != accountID || sec.Status != "active" {
		return domain.WebhookSecret{}, errors.New("invalid secret")
	}
	return sec, nil
}

func (s *MemoryStore) CreateForwardTarget(_ context.Context, accountID, targetType, configJSON string) (domain.ForwardTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj := domain.ForwardTarget{ID: uuid.NewString(), AccountID: accountID, TargetType: targetType, ConfigJSON: configJSON, CreatedAt: time.Now().UTC()}
	s.targets[obj.ID] = obj
	return obj, nil
}

func (s *MemoryStore) ListForwardTargets(_ context.Context, accountID string) ([]domain.ForwardTarget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.ForwardTarget
	for _, t := range s.targets {
		if t.AccountID == accountID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *MemoryStore) UpdateForwardTarget(_ context.Context, target domain.ForwardTarget) (domain.ForwardTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.targets[target.ID]
	if !ok || current.AccountID != target.AccountID {
		return domain.ForwardTarget{}, errors.New("forward target not found")
	}
	current.TargetType = target.TargetType
	current.ConfigJSON = target.ConfigJSON
	s.targets[target.ID] = current
	return current, nil
}

func (s *MemoryStore) DeleteForwardTarget(_ context.Context, accountID, targetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := s.targets[targetID]
	if !ok || target.AccountID != accountID {
		return errors.New("forward target not found")
	}
	delete(s.targets, targetID)
	return nil
}

func (s *MemoryStore) CreateIntegrationSecret(_ context.Context, secret domain.IntegrationSecret) (domain.IntegrationSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if secret.ID == "" {
		secret.ID = uuid.NewString()
	}
	secret.CreatedAt = time.Now().UTC()
	secret.UpdatedAt = secret.CreatedAt
	for _, existing := range s.integrationSecrets {
		if existing.AccountID == secret.AccountID && strings.EqualFold(existing.SecretKey, secret.SecretKey) {
			return domain.IntegrationSecret{}, errors.New("integration secret key already exists")
		}
	}
	s.integrationSecrets[secret.ID] = secret
	secret.SecretValue = ""
	return secret, nil
}

func (s *MemoryStore) ListIntegrationSecrets(_ context.Context, accountID string) ([]domain.IntegrationSecret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.IntegrationSecret{}
	for _, secret := range s.integrationSecrets {
		if secret.AccountID != accountID {
			continue
		}
		secret.SecretValue = ""
		out = append(out, secret)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) UpdateIntegrationSecret(_ context.Context, secret domain.IntegrationSecret) (domain.IntegrationSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.integrationSecrets[secret.ID]
	if !ok || current.AccountID != secret.AccountID {
		return domain.IntegrationSecret{}, errors.New("integration secret not found")
	}
	for id, existing := range s.integrationSecrets {
		if id == secret.ID {
			continue
		}
		if existing.AccountID == secret.AccountID && strings.EqualFold(existing.SecretKey, secret.SecretKey) {
			return domain.IntegrationSecret{}, errors.New("integration secret key already exists")
		}
	}
	current.SecretKey = secret.SecretKey
	current.Purpose = secret.Purpose
	if strings.TrimSpace(secret.SecretValue) != "" {
		current.SecretValue = secret.SecretValue
	}
	current.UpdatedAt = time.Now().UTC()
	s.integrationSecrets[secret.ID] = current
	current.SecretValue = ""
	return current, nil
}

func (s *MemoryStore) DeleteIntegrationSecret(_ context.Context, accountID, secretID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret, ok := s.integrationSecrets[secretID]
	if !ok || secret.AccountID != accountID {
		return errors.New("integration secret not found")
	}
	delete(s.integrationSecrets, secretID)
	return nil
}

func (s *MemoryStore) ResolveIntegrationSecretValue(_ context.Context, accountID, secretKey string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, secret := range s.integrationSecrets {
		if secret.AccountID == accountID && strings.EqualFold(secret.SecretKey, secretKey) {
			return secret.SecretValue, nil
		}
	}
	return "", errors.New("integration secret not found")
}

func (s *MemoryStore) CreateEvent(_ context.Context, e domain.WebhookEvent) (domain.WebhookEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.SourceEventID != "" {
		if id, ok := s.eventBySource[e.AccountID+"::"+e.SourceEventID]; ok {
			return domain.WebhookEvent{}, errors.New("duplicate source_event_id: " + id)
		}
	}
	e.ID = uuid.NewString()
	e.CreatedAt = time.Now().UTC()
	s.events[e.ID] = e
	if e.SourceEventID != "" {
		s.eventBySource[e.AccountID+"::"+e.SourceEventID] = e.ID
	}
	return e, nil
}

func (s *MemoryStore) UpdateEventStatus(_ context.Context, eventID, status, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.events[eventID]
	if !ok {
		return errors.New("event not found")
	}
	e.Status = status
	e.ActionSelected = action
	s.events[eventID] = e
	return nil
}

func (s *MemoryStore) UpdateEventProcessedText(_ context.Context, eventID, processedText string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.events[eventID]
	if !ok {
		return errors.New("event not found")
	}
	e.ProcessedText = processedText
	s.events[eventID] = e
	return nil
}

func (s *MemoryStore) GetEvent(_ context.Context, accountID, eventID string) (domain.WebhookEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev, ok := s.events[eventID]
	if !ok || ev.AccountID != accountID {
		return domain.WebhookEvent{}, errors.New("event not found")
	}
	return ev, nil
}

func (s *MemoryStore) ListEvents(_ context.Context, accountID string, limit int) ([]domain.WebhookEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.WebhookEvent, 0, limit)
	for _, e := range s.events {
		if e.AccountID == accountID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) FindEventBySourceEventID(_ context.Context, accountID, sourceEventID string) (domain.WebhookEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(sourceEventID) == "" {
		return domain.WebhookEvent{}, errors.New("source event id required")
	}
	id, ok := s.eventBySource[accountID+"::"+sourceEventID]
	if !ok {
		return domain.WebhookEvent{}, errors.New("event not found")
	}
	ev, ok := s.events[id]
	if !ok {
		return domain.WebhookEvent{}, errors.New("event not found")
	}
	return ev, nil
}

func (s *MemoryStore) ListEventsByTag(_ context.Context, accountID, tag string, limit int) ([]domain.WebhookEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.WebhookEvent
	for _, e := range s.events {
		if e.AccountID == accountID && strings.Contains(e.TagsJSON, `"`+tag+`"`) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) UpdateEventTags(_ context.Context, eventID, tagsJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev, ok := s.events[eventID]
	if !ok {
		return errors.New("event not found")
	}
	ev.TagsJSON = tagsJSON
	s.events[eventID] = ev
	return nil
}

func (s *MemoryStore) CreateTypeSignature(_ context.Context, sig domain.WebhookTypeSignature) (domain.WebhookTypeSignature, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sig.ID == "" {
		sig.ID = uuid.NewString()
	}
	if sig.Version <= 0 {
		sig.Version = 1
	}
	if sig.ConfidenceThreshold <= 0 {
		sig.ConfidenceThreshold = 0.75
	}
	sig.CreatedAt = time.Now().UTC()
	s.signatures[sig.ID] = sig
	return sig, nil
}

func (s *MemoryStore) ListTypeSignatures(_ context.Context, accountID string) ([]domain.WebhookTypeSignature, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.WebhookTypeSignature{}
	for _, sig := range s.signatures {
		if sig.AccountID == accountID && sig.Enabled {
			out = append(out, sig)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) GetLatestCandidateSignature(_ context.Context, accountID, typeKey string) (domain.WebhookTypeSignature, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best domain.WebhookTypeSignature
	found := false
	for _, sig := range s.signatures {
		if sig.AccountID != accountID || sig.TypeKey != typeKey {
			continue
		}
		if !strings.Contains(strings.ToLower(sig.Source), "llm_candidate") && !strings.Contains(strings.ToLower(sig.Source), "autopromote") {
			continue
		}
		if !found || sig.CreatedAt.After(best.CreatedAt) {
			best = sig
			found = true
		}
	}
	if !found {
		return domain.WebhookTypeSignature{}, errors.New("candidate signature not found")
	}
	return best, nil
}

func (s *MemoryStore) SetTypeSignatureEnabled(_ context.Context, signatureID string, enabled bool, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sig, ok := s.signatures[signatureID]
	if !ok {
		return errors.New("signature not found")
	}
	sig.Enabled = enabled
	if strings.TrimSpace(source) != "" {
		sig.Source = source
	}
	s.signatures[signatureID] = sig
	return nil
}

func (s *MemoryStore) CreateTransform(_ context.Context, tr domain.WebhookTransform) (domain.WebhookTransform, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tr.ID == "" {
		tr.ID = uuid.NewString()
	}
	if tr.Version <= 0 {
		tr.Version = 1
	}
	if tr.Status == "" {
		tr.Status = "pending"
	}
	tr.CreatedAt = time.Now().UTC()
	s.transforms[tr.ID] = tr
	return tr, nil
}

func (s *MemoryStore) ListTransforms(_ context.Context, accountID, typeKey string) ([]domain.WebhookTransform, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.WebhookTransform{}
	for _, tr := range s.transforms {
		if tr.AccountID != accountID {
			continue
		}
		if typeKey != "" && tr.TypeKey != typeKey {
			continue
		}
		out = append(out, tr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out, nil
}

func (s *MemoryStore) GetActiveTransform(_ context.Context, accountID, typeKey string) (domain.WebhookTransform, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best domain.WebhookTransform
	found := false
	for _, tr := range s.transforms {
		if tr.AccountID == accountID && tr.TypeKey == typeKey && tr.Status == "active" {
			if !found || tr.Version > best.Version {
				best = tr
				found = true
			}
		}
	}
	if !found {
		return domain.WebhookTransform{}, errors.New("active transform not found")
	}
	return best, nil
}

func (s *MemoryStore) GetLatestTransformByStatus(_ context.Context, accountID, typeKey, status string) (domain.WebhookTransform, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best domain.WebhookTransform
	found := false
	for _, tr := range s.transforms {
		if tr.AccountID != accountID || tr.TypeKey != typeKey {
			continue
		}
		if tr.Status != status {
			continue
		}
		if !found || tr.Version > best.Version {
			best = tr
			found = true
		}
	}
	if !found {
		return domain.WebhookTransform{}, errors.New("transform not found")
	}
	return best, nil
}

func (s *MemoryStore) SetTransformStatus(_ context.Context, transformID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tr, ok := s.transforms[transformID]
	if !ok {
		return errors.New("transform not found")
	}
	tr.Status = status
	s.transforms[transformID] = tr
	return nil
}

func (s *MemoryStore) LogTransformRun(_ context.Context, run domain.TransformRun) (domain.TransformRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	run.CreatedAt = time.Now().UTC()
	s.runs[run.ID] = run
	return run, nil
}

func (s *MemoryStore) UpsertMasterPromptPolicy(_ context.Context, accountID, promptText, updatedBy string) (domain.MasterPromptPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	cur, ok := s.policiesByAccount[accountID]
	if ok {
		cur.PromptText = promptText
		cur.UpdatedBy = updatedBy
		cur.UpdatedAt = now
		s.policiesByAccount[accountID] = cur
		return cur, nil
	}
	p := domain.MasterPromptPolicy{
		AccountID:  accountID,
		PromptText: promptText,
		UpdatedBy:  updatedBy,
		UpdatedAt:  now,
		CreatedAt:  now,
	}
	s.policiesByAccount[accountID] = p
	return p, nil
}

func (s *MemoryStore) GetMasterPromptPolicy(_ context.Context, accountID string) (domain.MasterPromptPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policiesByAccount[accountID]
	if !ok {
		return domain.MasterPromptPolicy{}, errors.New("policy not found")
	}
	return p, nil
}

func (s *MemoryStore) CreateWebhookSkill(_ context.Context, skill domain.WebhookSkill) (domain.WebhookSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if skill.ID == "" {
		skill.ID = uuid.NewString()
	}
	if skill.Priority == 0 {
		skill.Priority = 100
	}
	if skill.MemoryWriteMode == "" {
		skill.MemoryWriteMode = "update_or_insert"
	}
	skill.CreatedAt = time.Now().UTC()
	s.skills[skill.ID] = skill
	return skill, nil
}

func (s *MemoryStore) ListWebhookSkills(_ context.Context, accountID, typeKey string) ([]domain.WebhookSkill, error) {
	return s.listWebhookSkills(accountID, typeKey, false), nil
}

func (s *MemoryStore) ListWebhookSkillsIncludingDisabled(_ context.Context, accountID, typeKey string) ([]domain.WebhookSkill, error) {
	return s.listWebhookSkills(accountID, typeKey, true), nil
}

func (s *MemoryStore) listWebhookSkills(accountID, typeKey string, includeDisabled bool) []domain.WebhookSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.WebhookSkill{}
	for _, sk := range s.skills {
		if sk.AccountID != accountID {
			continue
		}
		if sk.TypeKey != typeKey {
			continue
		}
		if !includeDisabled && !sk.Enabled {
			continue
		}
		out = append(out, sk)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Priority < out[j].Priority
	})
	return out
}

func (s *MemoryStore) UpdateWebhookSkill(_ context.Context, skill domain.WebhookSkill) (domain.WebhookSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.skills[skill.ID]
	if !ok || current.AccountID != skill.AccountID {
		return domain.WebhookSkill{}, errors.New("skill not found")
	}
	if skill.Priority == 0 {
		skill.Priority = 100
	}
	if skill.MemoryWriteMode == "" {
		skill.MemoryWriteMode = "update_or_insert"
	}
	skill.CreatedAt = current.CreatedAt
	s.skills[skill.ID] = skill
	return skill, nil
}

func (s *MemoryStore) UpsertAutoPromoteState(_ context.Context, state domain.AutoPromoteState) (domain.AutoPromoteState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := state.AccountID + "::" + state.TypeKey
	now := time.Now().UTC()
	cur, ok := s.autoStates[key]
	if !ok {
		state.CreatedAt = now
		state.UpdatedAt = now
		s.autoStates[key] = state
		return state, nil
	}
	if state.Status != "" {
		cur.Status = state.Status
	}
	cur.ValidatedCount = state.ValidatedCount
	cur.ShadowTotal = state.ShadowTotal
	cur.ShadowSuccess = state.ShadowSuccess
	cur.LastConfidence = state.LastConfidence
	cur.LastReason = state.LastReason
	cur.UpdatedAt = now
	s.autoStates[key] = cur
	return cur, nil
}

func (s *MemoryStore) GetAutoPromoteState(_ context.Context, accountID, typeKey string) (domain.AutoPromoteState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := accountID + "::" + typeKey
	st, ok := s.autoStates[key]
	if !ok {
		return domain.AutoPromoteState{}, errors.New("autopromote state not found")
	}
	return st, nil
}

func (s *MemoryStore) UpsertBYOKConfig(_ context.Context, cfg domain.BYOKProviderConfig) (domain.BYOKProviderConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg.ID == "" {
		cfg.ID = uuid.NewString()
	}
	cfg.CreatedAt = time.Now().UTC()
	if s.byokConfigs == nil {
		s.byokConfigs = map[string]domain.BYOKProviderConfig{}
	}
	key := cfg.AccountID + "::" + cfg.Provider
	s.byokConfigs[key] = cfg
	return cfg, nil
}

func (s *MemoryStore) GetBYOKConfig(_ context.Context, accountID, provider string) (domain.BYOKProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := accountID + "::" + provider
	cfg, ok := s.byokConfigs[key]
	if !ok {
		return domain.BYOKProviderConfig{}, errors.New("byok config not found")
	}
	return cfg, nil
}

func (s *MemoryStore) GetDefaultBYOKConfig(_ context.Context, accountID string) (domain.BYOKProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, cfg := range s.byokConfigs {
		if cfg.AccountID == accountID && cfg.IsDefault {
			return cfg, nil
		}
	}
	for _, cfg := range s.byokConfigs {
		if cfg.AccountID == accountID {
			return cfg, nil
		}
	}
	return domain.BYOKProviderConfig{}, errors.New("no byok config found")
}

func (s *MemoryStore) ListBYOKConfigs(_ context.Context, accountID string) ([]domain.BYOKProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.BYOKProviderConfig
	for _, cfg := range s.byokConfigs {
		if cfg.AccountID == accountID {
			out = append(out, cfg)
		}
	}
	return out, nil
}

func slugFromEmail(email string) string {
	for i, c := range email {
		if c == '@' {
			return email[:i]
		}
	}
	return email
}
