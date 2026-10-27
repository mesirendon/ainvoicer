package extraction

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

type FixtureExtractor struct {
	samples fs.FS
}

func NewFixtureExtractor(dir string) *FixtureExtractor {
	return &FixtureExtractor{samples: os.DirFS(dir)}
}

func (f *FixtureExtractor) Extract(_ context.Context, fileName string, _ []byte) (ExtractedInvoice, error) {
	base := path.Base(strings.ReplaceAll(fileName, "\\", "/"))
	name := strings.TrimSuffix(base, path.Ext(base)) + ".expected.json"
	data, err := fs.ReadFile(f.samples, name)
	if errors.Is(err, fs.ErrNotExist) {
		return ExtractedInvoice{}, fmt.Errorf("%w: fixture mode has no pre-extracted data for %q; set ANTHROPIC_API_KEY to extract arbitrary files", ErrInvalid, base)
	}
	if err != nil {
		return ExtractedInvoice{}, fmt.Errorf("read fixture %s: %w", name, err)
	}
	return Parse(data)
}
