package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"agenthook.store/internal/domain"
)

type TypeResolver struct {
	Store             domain.Store
	Groq              domain.TypeClassifier
	Cerebras          domain.TypeClassifier
	AutoPromoter      *AutoPromoter
	DeterministicOnly map[string]struct{}
}

func NewTypeResolver(store domain.Store, groq, cerebras domain.TypeClassifier) *TypeResolver {
	return &TypeResolver{Store: store, Groq: groq, Cerebras: cerebras}
}

func (r *TypeResolver) Resolve(ctx context.Context, accountID, payload string, headers map[string]string) (domain.TypeResolution, error) {
	fp, err := BuildFingerprint(payload)
	if err != nil {
		return domain.TypeResolution{}, err
	}
	sigs, err := r.Store.ListTypeSignatures(ctx, accountID)
	if err != nil {
		return domain.TypeResolution{}, err
	}
	if det, ok := bestDeterministicMatch(fp, headers, sigs); ok {
		if r.AutoPromoter != nil {
			_ = r.AutoPromoter.ObserveDeterministicHit(ctx, accountID, det.TypeKey, det.Confidence, det.Reason)
		}
		return det, nil
	}
	if r.Groq == nil || r.Cerebras == nil {
		return domain.TypeResolution{TypeKey: "unknown", Confidence: 0, Source: "none", Reason: "no deterministic match and llm classifiers unavailable", ManualReview: false}, nil
	}
	// Run provider classification in parallel so resolver latency stays bounded.
	classifyCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		groqRes domain.TypeResolution
		cerRes  domain.TypeResolution
		gErr    error
		cErr    error
		wg      sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		groqRes, gErr = r.Groq.ClassifyType(classifyCtx, payload, headers, fp.Summary)
	}()
	go func() {
		defer wg.Done()
		cerRes, cErr = r.Cerebras.ClassifyType(classifyCtx, payload, headers, fp.Summary)
	}()
	wg.Wait()
	if gErr != nil || cErr != nil {
		return domain.TypeResolution{TypeKey: "unknown", Confidence: 0, Source: "llm_error", Reason: fmt.Sprintf("provider error (groq=%v cerebras=%v)", gErr, cErr), ManualReview: false}, nil
	}
	if strings.EqualFold(strings.TrimSpace(groqRes.TypeKey), strings.TrimSpace(cerRes.TypeKey)) && groqRes.TypeKey != "" {
		conf := (groqRes.Confidence + cerRes.Confidence) / 2
		res := groqRes
		res.Confidence = conf
		res.Source = "llm_agreement"
		res.Reason = "groq and cerebras agreement"
		res.ManualReview = false
		_ = persistLLMCandidate(ctx, r.Store, accountID, res, fp)
		if r.AutoPromoter != nil {
			_ = r.AutoPromoter.ObserveCandidate(ctx, accountID, res)
		}
		return res, nil
	}
	// For disagreement, pick higher confidence and proceed.
	picked := groqRes
	if cerRes.Confidence > groqRes.Confidence {
		picked = cerRes
	}
	if strings.TrimSpace(picked.TypeKey) == "" {
		picked.TypeKey = "unknown"
	}
	picked.Source = "llm_disagreement_resolved"
	picked.Reason = fmt.Sprintf("groq=%s(%.2f) cerebras=%s(%.2f)", groqRes.TypeKey, groqRes.Confidence, cerRes.TypeKey, cerRes.Confidence)
	picked.ManualReview = false
	if strings.TrimSpace(picked.TypeKey) != "" && picked.TypeKey != "unknown" {
		_ = persistLLMCandidate(ctx, r.Store, accountID, picked, fp)
		if r.AutoPromoter != nil {
			_ = r.AutoPromoter.ObserveCandidate(ctx, accountID, picked)
		}
	}
	return picked, nil
}

func bestDeterministicMatch(fp PayloadFingerprint, headers map[string]string, sigs []domain.WebhookTypeSignature) (domain.TypeResolution, bool) {
	if len(sigs) == 0 {
		return domain.TypeResolution{}, false
	}
	keySet := map[string]struct{}{}
	for _, k := range fp.KeyPaths {
		keySet[k] = struct{}{}
	}
	hdr := asStringMap(headers)
	type scored struct {
		sig   domain.WebhookTypeSignature
		score float64
	}
	all := make([]scored, 0, len(sigs))
	for _, sig := range sigs {
		required := parseJSONStringArray(sig.RequiredKeysJSON)
		shape := parseJSONStringMap(sig.ShapeHintsJSON)
		headersExpected := parseJSONStringMap(sig.HeaderHintsJSON)

		keyScore := 0.0
		if len(required) > 0 {
			matched := 0
			for _, rk := range required {
				if mapHasPath(keySet, rk) {
					matched++
				}
			}
			keyScore = float64(matched) / float64(len(required))
		}
		shapeScore := 0.0
		if len(shape) > 0 {
			matched := 0
			for p, exp := range shape {
				if shapeMatch(exp, fp.Shapes[p]) {
					matched++
				}
			}
			shapeScore = float64(matched) / float64(len(shape))
		}
		hdrScore := 0.0
		if len(headersExpected) > 0 {
			matched := 0
			for hk, hv := range headersExpected {
				if strings.Contains(strings.ToLower(hdr[strings.ToLower(hk)]), strings.ToLower(hv)) {
					matched++
				}
			}
			hdrScore = float64(matched) / float64(len(headersExpected))
		}
		combined := (keyScore * 0.7) + (shapeScore * 0.2) + (hdrScore * 0.1)
		all = append(all, scored{sig: sig, score: combined})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	best := all[0]
	if best.score >= best.sig.ConfidenceThreshold {
		return domain.TypeResolution{TypeKey: best.sig.TypeKey, Confidence: best.score, Source: "deterministic", Reason: "signature match", ManualReview: false}, true
	}
	return domain.TypeResolution{}, false
}

func persistLLMCandidate(ctx context.Context, st domain.Store, accountID string, res domain.TypeResolution, fp PayloadFingerprint) error {
	keys, _ := json.Marshal(fp.KeyPaths)
	shape, _ := json.Marshal(fp.Shapes)
	_, _ = st.CreateTypeSignature(ctx, domain.WebhookTypeSignature{
		AccountID:           accountID,
		TypeKey:             res.TypeKey,
		Version:             1,
		RequiredKeysJSON:    string(keys),
		ShapeHintsJSON:      string(shape),
		HeaderHintsJSON:     `{}`,
		ConfidenceThreshold: 0.8,
		Enabled:             false,
		Source:              "llm_candidate",
	})
	if len(res.TransformTemplate) > 0 {
		dsl, _ := json.Marshal(res.TransformTemplate)
		_, _ = st.CreateTransform(ctx, domain.WebhookTransform{
			AccountID:              accountID,
			TypeKey:                res.TypeKey,
			Version:                1,
			Engine:                 "dsl",
			DSLText:                string(dsl),
			DeterministicTestsJSON: `[]`,
			Status:                 "pending",
		})
	}
	return nil
}
