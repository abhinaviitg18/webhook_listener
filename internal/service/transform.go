package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"hookweb.club/internal/domain"
)

type TransformService struct {
	Store domain.Store
}

func NewTransformService(store domain.Store) *TransformService {
	return &TransformService{Store: store}
}

func (t *TransformService) Apply(ctx context.Context, accountID, typeKey, payload string) (domain.TransformResult, error) {
	tr, err := t.Store.GetActiveTransform(ctx, accountID, typeKey)
	if err != nil {
		return domain.TransformResult{CanonicalPayload: payload, Engine: "none", Version: 0}, nil
	}
	started := time.Now()
	var canonical string
	switch strings.ToLower(strings.TrimSpace(tr.Engine)) {
	case "dsl":
		canonical, err = applyDSL(payload, tr.DSLText)
	case "wasm":
		canonical, err = runWASMTransform(ctx, tr.WASMBlobRef, payload)
	default:
		err = fmt.Errorf("unsupported transform engine: %s", tr.Engine)
	}
	dur := time.Since(started).Milliseconds()
	run := domain.TransformRun{
		EventID:          "",
		AccountID:        accountID,
		TypeKey:          typeKey,
		TransformVersion: tr.Version,
		DurationMS:       dur,
		ResultHash:       hashPayload(canonical),
	}
	if err != nil {
		run.ErrorText = err.Error()
		_, _ = t.Store.LogTransformRun(ctx, run)
		return domain.TransformResult{}, err
	}
	_, _ = t.Store.LogTransformRun(ctx, run)
	return domain.TransformResult{CanonicalPayload: canonical, Engine: tr.Engine, Version: tr.Version}, nil
}

type dslSpec struct {
	Extract   map[string]string      `json:"extract"`
	Constants map[string]interface{} `json:"constants"`
	DropNull  bool                   `json:"drop_null"`
}

func applyDSL(payload, dslRaw string) (string, error) {
	if strings.TrimSpace(dslRaw) == "" {
		return payload, nil
	}
	var src interface{}
	if err := json.Unmarshal([]byte(payload), &src); err != nil {
		return "", err
	}
	var spec dslSpec
	if err := json.Unmarshal([]byte(dslRaw), &spec); err != nil {
		return "", fmt.Errorf("invalid dsl json: %w", err)
	}
	out := map[string]interface{}{}
	for k, p := range spec.Extract {
		val, ok := jsonPathGet(src, p)
		if !ok && spec.DropNull {
			continue
		}
		out[k] = val
	}
	for k, v := range spec.Constants {
		out[k] = v
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func jsonPathGet(root interface{}, path string) (interface{}, bool) {
	path = strings.TrimSpace(path)
	if path == "$" {
		return root, true
	}
	if !strings.HasPrefix(path, "$") {
		return nil, false
	}
	cur := root
	parts := strings.Split(strings.TrimPrefix(path, "$."), ".")
	for _, p := range parts {
		if p == "" {
			continue
		}
		if strings.Contains(p, "[") && strings.HasSuffix(p, "]") {
			left := p[:strings.Index(p, "[")]
			idxRaw := p[strings.Index(p, "[")+1 : len(p)-1]
			if left != "" {
				m, ok := cur.(map[string]interface{})
				if !ok {
					return nil, false
				}
				cur, ok = m[left]
				if !ok {
					return nil, false
				}
			}
			arr, ok := cur.([]interface{})
			if !ok {
				return nil, false
			}
			idx, err := strconv.Atoi(idxRaw)
			if err != nil || idx < 0 || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
			continue
		}
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		nxt, ok := m[p]
		if !ok {
			return nil, false
		}
		cur = nxt
	}
	return cur, true
}

func runWASMTransform(ctx context.Context, wasmBlobRef, payload string) (string, error) {
	if strings.TrimSpace(wasmBlobRef) == "" {
		return "", errors.New("missing wasm_blob_ref")
	}
	code, err := os.ReadFile(wasmBlobRef)
	if err != nil {
		return "", err
	}
	ctrx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	r := wazero.NewRuntime(ctrx)
	defer func() { _ = r.Close(ctrx) }()
	_, err = wasi_snapshot_preview1.Instantiate(ctrx, r)
	if err != nil {
		return "", err
	}
	stdin := bytes.NewBufferString(payload)
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	cfg := wazero.NewModuleConfig().WithStdin(stdin).WithStdout(stdout).WithStderr(stderr)
	_, err = r.InstantiateWithConfig(ctrx, code, cfg)
	if err != nil {
		return "", fmt.Errorf("wasm exec failed: %w: %s", err, stderr.String())
	}
	out, err := io.ReadAll(stdout)
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return "", fmt.Errorf("wasm returned empty output")
	}
	// ensure returned output is json
	var tmp interface{}
	if err := json.Unmarshal(out, &tmp); err != nil {
		return "", fmt.Errorf("wasm output must be valid json: %w", err)
	}
	return string(bytes.TrimSpace(out)), nil
}

func hashPayload(in string) string {
	s := sha256.Sum256([]byte(in))
	return hex.EncodeToString(s[:])
}
