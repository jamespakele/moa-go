package tavily

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jpakele/moa-go/internal/config"
	"github.com/jpakele/moa-go/internal/search"
)

func TestTavilySearchRequestAndResponse(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "")

	var gotBody searchRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/search" {
			t.Errorf("expected path /search, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		resp := searchResponse{
			Answer: "The answer is 42.",
			Results: []responseResult{
				{Title: "First", URL: "https://example.com/1", Content: "content one"},
				{Title: "Second", URL: "https://example.com/2", Content: "content two"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	cfg := config.SearchConfig{
		Provider: "tavily",
		APIKey:   "test-api-key",
	}

	// Use the concrete client but point it at the test server.
	c := New(cfg).(*client)
	c.endpoint = server.URL + "/search"

	result, err := c.Search(context.Background(), "what is the meaning of life")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if gotBody.APIKey != "test-api-key" {
		t.Errorf("expected api_key test-api-key, got %s", gotBody.APIKey)
	}
	if gotBody.Query != "what is the meaning of life" {
		t.Errorf("expected query 'what is the meaning of life', got %s", gotBody.Query)
	}
	if gotBody.SearchDepth != "advanced" {
		t.Errorf("expected search_depth advanced, got %s", gotBody.SearchDepth)
	}
	if gotBody.MaxResults != 5 {
		t.Errorf("expected max_results 5, got %d", gotBody.MaxResults)
	}
	if !gotBody.IncludeAnswer {
		t.Errorf("expected include_answer true")
	}

	if result.Answer != "The answer is 42." {
		t.Errorf("expected answer 'The answer is 42.', got %s", result.Answer)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(result.Sources))
	}
	if result.Sources[0].Title != "First" || result.Sources[0].URL != "https://example.com/1" || result.Sources[0].Content != "content one" {
		t.Errorf("unexpected first source: %+v", result.Sources[0])
	}
	if result.Sources[1].Title != "Second" || result.Sources[1].URL != "https://example.com/2" || result.Sources[1].Content != "content two" {
		t.Errorf("unexpected second source: %+v", result.Sources[1])
	}
}

func TestTavilySearchMissingKey(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "")
	cfg := config.SearchConfig{Provider: "tavily"}
	c := New(cfg).(*client)
	_, err := c.Search(context.Background(), "query")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewSearcherReturnsTavily(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "")
	cfg := config.SearchConfig{Provider: "tavily", APIKey: "k"}
	s, err := search.NewSearcher(cfg)
	if err != nil {
		t.Fatalf("NewSearcher error: %v", err)
	}
	if s == nil {
		t.Fatal("NewSearcher returned nil Searcher")
	}
	// Smoke-check that it is the Tavily implementation.
	_, ok := s.(*client)
	if !ok {
		t.Fatalf("expected *tavily.client, got %T", s)
	}
}
