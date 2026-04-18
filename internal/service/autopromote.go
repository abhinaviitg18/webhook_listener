package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"hookweb.club/internal/domain"
)

type AutoPromoteConfig struct {
	Enabled           bool
	MinConfidence     float64
	ValidatedToShadow int
	ShadowToActive    int
	MinSuccessRate    float64
	DeterministicOnly map[string]struct{}
}

type AutoPromoter struct {
	Store domain.Store
	Cfg   AutoPromoteConfig
}

func NewAutoPromoter(store domain.Store, cfg AutoPromoteConfig) *AutoPromoter {
	return &AutoPromoter{Store: store, Cfg: cfg}
}

func (a *AutoPromoter) ObserveCandidate(ctx context.Context, accountID string, res domain.TypeResolution) error {
	if a == nil || !a.Cfg.Enabled || a.Store == nil {
		return nil
	}
	typeKey := strings.TrimSpace(res.TypeKey)
	if typeKey == "" || typeKey == "unknown" || isDeterministicOnly(typeKey, a.Cfg.DeterministicOnly) {
		return nil
	}
	st, _ := a.loadState(ctx, accountID, typeKey)
	st.LastConfidence = res.Confidence
	st.LastReason = strings.TrimSpace(res.Reason)
	if st.Status == "" {
		st.Status = "validated"
	}
	if res.Confidence >= a.Cfg.MinConfidence {
		st.ValidatedCount++
	}
	if st.Status == "validated" && st.ValidatedCount >= maxInt(a.Cfg.ValidatedToShadow, 1) {
		if sig, err := a.Store.GetLatestCandidateSignature(ctx, accountID, typeKey); err == nil {
			_ = a.Store.SetTypeSignatureEnabled(ctx, sig.ID, true, "autopromote_shadow")
		}
		if tr, err := a.Store.GetLatestTransformByStatus(ctx, accountID, typeKey, "pending"); err == nil {
			_ = a.Store.SetTransformStatus(ctx, tr.ID, "shadow")
		}
		st.Status = "shadow"
		st.ShadowTotal = 0
		st.ShadowSuccess = 0
		st.ValidatedCount = 0
	}
	_, err := a.Store.UpsertAutoPromoteState(ctx, st)
	return err
}

func (a *AutoPromoter) ObserveDeterministicHit(ctx context.Context, accountID, typeKey string, confidence float64, reason string) error {
	if a == nil || !a.Cfg.Enabled || a.Store == nil || isDeterministicOnly(typeKey, a.Cfg.DeterministicOnly) {
		return nil
	}
	st, err := a.Store.GetAutoPromoteState(ctx, accountID, typeKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return nil
	}
	if st.Status != "shadow" {
		return nil
	}
	st.ShadowTotal++
	st.ShadowSuccess++
	st.LastConfidence = confidence
	st.LastReason = reason

	if st.ShadowTotal >= maxInt(a.Cfg.ShadowToActive, 1) {
		successRate := float64(st.ShadowSuccess) / float64(maxInt(st.ShadowTotal, 1))
		if successRate >= a.Cfg.MinSuccessRate {
			if tr, trErr := a.Store.GetLatestTransformByStatus(ctx, accountID, typeKey, "shadow"); trErr == nil {
				_ = a.Store.SetTransformStatus(ctx, tr.ID, "active")
			} else if pending, pErr := a.Store.GetLatestTransformByStatus(ctx, accountID, typeKey, "pending"); pErr == nil {
				_ = a.Store.SetTransformStatus(ctx, pending.ID, "active")
			}
			if sig, sigErr := a.Store.GetLatestCandidateSignature(ctx, accountID, typeKey); sigErr == nil {
				_ = a.Store.SetTypeSignatureEnabled(ctx, sig.ID, true, "autopromote_active")
			}
			st.Status = "active"
		} else {
			if sig, sigErr := a.Store.GetLatestCandidateSignature(ctx, accountID, typeKey); sigErr == nil {
				_ = a.Store.SetTypeSignatureEnabled(ctx, sig.ID, false, "autopromote_rollback")
			}
			if tr, trErr := a.Store.GetLatestTransformByStatus(ctx, accountID, typeKey, "shadow"); trErr == nil {
				_ = a.Store.SetTransformStatus(ctx, tr.ID, "pending")
			}
			st.Status = "validated"
			st.ValidatedCount = 0
			st.ShadowTotal = 0
			st.ShadowSuccess = 0
		}
	}
	_, err = a.Store.UpsertAutoPromoteState(ctx, st)
	return err
}

func (a *AutoPromoter) loadState(ctx context.Context, accountID, typeKey string) (domain.AutoPromoteState, error) {
	st, err := a.Store.GetAutoPromoteState(ctx, accountID, typeKey)
	if err == nil {
		return st, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		// in-memory store and some drivers return plain errors; initialize state anyway
		return domain.AutoPromoteState{
			AccountID: accountID,
			TypeKey:   typeKey,
			Status:    "validated",
		}, nil
	}
	return domain.AutoPromoteState{
		AccountID: accountID,
		TypeKey:   typeKey,
		Status:    "validated",
	}, nil
}

func isDeterministicOnly(typeKey string, deterministicOnly map[string]struct{}) bool {
	_, ok := deterministicOnly[strings.ToLower(strings.TrimSpace(typeKey))]
	return ok
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
