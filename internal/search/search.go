// Package search defines the web-search backend interface used by the verifier.
package search

import (
	"context"
	"fmt"

	"github.com/jpakele/moa-go/internal/config"
)

// Source is a single web-search result.
type Source struct {
	Title   string
	URL     string
	Content string
}

// Result is the aggregated answer and the supporting sources.
type Result struct {
	Answer  string
	Sources []Source
}

// Searcher abstracts a web-search backend.
type Searcher interface {
	Search(ctx context.Context, query string) (Result, error)
}

// constructors holds provider-specific Searcher factories, registered by
// backend subpackages (e.g. internal/search/tavily) at import time.
var constructors = map[string]func(config.SearchConfig) Searcher{}

// Register adds a Searcher factory for the given provider name.
func Register(provider string, ctor func(config.SearchConfig) Searcher) {
	if _, exists := constructors[provider]; exists {
		panic(fmt.Sprintf("search provider %q already registered", provider))
	}
	constructors[provider] = ctor
}

// NewSearcher returns a Searcher for the configured provider.
// Supported providers: "tavily" (requires tavily package to be imported).
func NewSearcher(cfg config.SearchConfig) (Searcher, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("search provider not configured")
	}
	ctor, ok := constructors[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown search provider: %s", cfg.Provider)
	}
	return ctor(cfg), nil
}
