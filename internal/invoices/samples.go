package invoices

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

func (s *Service) ListSamples(_ context.Context) ([]Sample, error) {
	names, err := fs.Glob(os.DirFS(s.SamplesDir), "*.pdf")
	if err != nil {
		return nil, fmt.Errorf("list samples: %w", err)
	}
	samples := make([]Sample, 0, len(names))
	for _, n := range names {
		samples = append(samples, Sample{Name: strings.TrimSuffix(n, ".pdf"), FileName: n})
	}
	return samples, nil
}

func (s *Service) ProcessSample(ctx context.Context, name string) (Detail, bool, error) {
	fileName := name + ".pdf"
	if !fs.ValidPath(fileName) || strings.Contains(name, "/") {
		return Detail{}, false, ErrNotFound
	}
	pdf, err := fs.ReadFile(os.DirFS(s.SamplesDir), fileName)
	if errors.Is(err, fs.ErrNotExist) {
		return Detail{}, false, ErrNotFound
	}
	if err != nil {
		return Detail{}, false, fmt.Errorf("read sample %s: %w", fileName, err)
	}
	return s.Process(ctx, fileName, pdf)
}
