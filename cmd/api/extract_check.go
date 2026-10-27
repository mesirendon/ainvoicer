package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"ap-agent/internal/config"
	"ap-agent/internal/extraction"
)

type extractor interface {
	Extract(ctx context.Context, fileName string, pdf []byte) (extraction.ExtractedInvoice, error)
}

func newExtractor(cfg config.Config) extractor {
	if cfg.FixtureMode() {
		return extraction.NewFixtureExtractor(cfg.SamplesDir)
	}
	return extraction.NewClaudeExtractor(cfg.AnthropicAPIKey, cfg.AnthropicModel)
}

func extractCheck(cfg config.Config, path string) error {
	dir := os.DirFS(filepath.Dir(path))
	info, err := fs.Stat(dir, filepath.Base(path))
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if limit := int64(cfg.ExtractMaxFileMB) << 20; info.Size() > limit {
		return fmt.Errorf("%s is %d bytes, larger than EXTRACT_MAX_FILE_MB=%d", path, info.Size(), cfg.ExtractMaxFileMB)
	}
	pdf, err := fs.ReadFile(dir, filepath.Base(path))
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ExtractTimeoutMS)*time.Millisecond)
	defer cancel()

	mode := "claude (" + cfg.AnthropicModel + ")"
	if cfg.FixtureMode() {
		mode = "fixture (no ANTHROPIC_API_KEY)"
	}
	fmt.Fprintf(os.Stderr, "extracting %s with %s\n", path, mode)

	inv, err := newExtractor(cfg).Extract(ctx, filepath.Base(path), pdf)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(out))
	return nil
}
