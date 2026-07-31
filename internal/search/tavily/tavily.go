// Package tavily implements the Tavily web-search backend.
package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jpakele/moa-go/internal/config"
	"github.com/jpakele/moa-go/internal/search"
)

const (
	defaultEndpoint = "https://api.tavily.com/search"
	defaultTimeout  = 30 * time.Second
)

// client implements search.Searcher for Tavily.
type client struct {
	cfg        config.SearchConfig
	httpClient *http.Client
	endpoint   string
}

func init() {
	search.Register("tavily", func(cfg config.SearchConfig) search.Searcher {
		return New(cfg)
	})
}

// New returns a Tavily-backed Searcher using the provided configuration.
func New(cfg config.SearchConfig) search.Searcher {
	endpoint := defaultEndpoint
	if cfg.Provider != "" && cfg.Provider != "tavily" {
		// No custom endpoint support for Tavily; keep default.
	}
	return &client{
		cfg:        cfg,
		httpClient: &http.Client{},
		endpoint:   endpoint,
	}
}

type searchRequest struct {
	APIKey        string `json:"api_key"`
	Query         string `json:"query"`
	SearchDepth   string `json:"search_depth"`
	MaxResults    int    `json:"max_results"`
	IncludeAnswer bool   `json:"include_answer"`
}

type searchResponse struct {
	Answer  string           `json:"answer"`
	Results []responseResult `json:"results"`
}

type responseResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// Search queries Tavily for the given query and returns the answer + sources.
func (c *client) Search(ctx context.Context, query string) (search.Result, error) {
	key, err := resolveAPIKey(c.cfg)
	if err != nil {
		return search.Result{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	reqBody := searchRequest{
		APIKey:        key,
		Query:         query,
		SearchDepth:   "advanced",
		MaxResults:    5,
		IncludeAnswer: true,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return search.Result{}, fmt.Errorf("tavily: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return search.Result{}, fmt.Errorf("tavily: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return search.Result{}, fmt.Errorf("tavily: request failed: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return search.Result{}, fmt.Errorf("tavily: read response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return search.Result{}, fmt.Errorf("tavily: unexpected status %d: %s", httpResp.StatusCode, string(body))
	}

	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return search.Result{}, fmt.Errorf("tavily: decode response: %w", err)
	}

	sources := make([]search.Source, 0, len(resp.Results))
	for _, r := range resp.Results {
		sources = append(sources, search.Source{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
		})
	}

	return search.Result{
		Answer:  resp.Answer,
		Sources: sources,
	}, nil
}

// resolveAPIKey returns the Tavily API key from config or the TAVILY_API_KEY
// environment variable.
func resolveAPIKey(cfg config.SearchConfig) (string, error) {
	if cfg.APIKey != "" {
		return cfg.APIKey, nil
	}
	if key := os.Getenv("TAVILY_API_KEY"); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("tavily: api_key not configured and TAVILY_API_KEY not set")
}
