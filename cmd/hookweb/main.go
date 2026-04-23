package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"agenthook.store/internal/config"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/integrations"
	"agenthook.store/internal/service"
	"agenthook.store/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: agenthook <classify|transform> [flags]")
		os.Exit(2)
	}
	cmd := os.Args[1]
	switch cmd {
	case "classify":
		runClassify(os.Args[2:])
	case "transform":
		runTransform(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		os.Exit(2)
	}
}

func bootstrapStore() domain.Store {
	cfg := config.Load()
	if cfg.UseInMemoryStore {
		return store.NewMemoryStore()
	}
	s, err := store.NewMySQLStore(cfg.TiDBDSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mysql init failed:", err)
		os.Exit(1)
	}
	return s
}

func runClassify(args []string) {
	fs := flag.NewFlagSet("classify", flag.ExitOnError)
	accountID := fs.String("account", "local", "account id")
	fs.Parse(args)
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	st := bootstrapStore()
	cfg := config.Load()
	resolver := service.NewTypeResolver(
		st,
		integrations.NewProviderTypeClassifier("groq", cfg.GroqBaseURL, cfg.GroqAPIKey, cfg.GroqModel),
		integrations.NewProviderTypeClassifier("cerebras", cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, cfg.CerebrasModel),
	)
	res, err := resolver.Resolve(context.Background(), strings.TrimSpace(*accountID), string(payload), map[string]string{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("type=%s confidence=%.3f source=%s manual_review=%v reason=%s\n", res.TypeKey, res.Confidence, res.Source, res.ManualReview, res.Reason)
	if len(res.TransformTemplate) > 0 {
		fmt.Printf("extract_template=%v\n", res.TransformTemplate)
	}
}

func runTransform(args []string) {
	fs := flag.NewFlagSet("transform", flag.ExitOnError)
	accountID := fs.String("account", "local", "account id")
	typeKey := fs.String("type", "", "type key")
	fs.Parse(args)
	if strings.TrimSpace(*typeKey) == "" {
		fmt.Fprintln(os.Stderr, "--type is required")
		os.Exit(2)
	}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	st := bootstrapStore()
	ts := service.NewTransformService(st)
	out, err := ts.Apply(context.Background(), strings.TrimSpace(*accountID), strings.TrimSpace(*typeKey), string(payload))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(out.CanonicalPayload)
}
