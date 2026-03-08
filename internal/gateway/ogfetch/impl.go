package ogfetch

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/html"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

// Fetcher implements domain/gateway.OGFetcher using the standard HTTP client.
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a new OG metadata fetcher with the given HTTP client.
func NewFetcher(client *http.Client) *Fetcher {
	return &Fetcher{client: client}
}

// Fetch retrieves the Open Graph metadata from the target URL by parsing its HTML.
// Extracts og:title, og:description, og:image, and og:site_name meta tags.
func (f *Fetcher) Fetch(ctx context.Context, url string) (*entity.OGMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ogfetch.Fetch: create request: %w", err)
	}
	// Mimic a browser UA so sites don't block the request.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; short-url-og-fetcher/1.0)")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ogfetch.Fetch: http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ogfetch.Fetch: unexpected status %d", resp.StatusCode)
	}

	og := &entity.OGMetadata{}
	if err := parseOGTags(resp, og); err != nil {
		return nil, fmt.Errorf("ogfetch.Fetch: parse html: %w", err)
	}
	return og, nil
}

// parseOGTags tokenizes the HTML response and extracts OG meta tag values.
func parseOGTags(resp *http.Response, og *entity.OGMetadata) error {
	tokenizer := html.NewTokenizer(resp.Body)

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			// End of document — not necessarily an error.
			return nil

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()

			// Stop scanning once we leave <head>: OG tags live in <head>.
			if token.Data == "body" {
				return nil
			}

			if token.Data != "meta" {
				continue
			}

			var property, content string
			for _, attr := range token.Attr {
				switch strings.ToLower(attr.Key) {
				case "property":
					property = strings.ToLower(attr.Val)
				case "content":
					content = attr.Val
				}
			}

			switch property {
			case "og:title":
				og.Title = content
			case "og:description":
				og.Description = content
			case "og:image":
				og.Image = content
			case "og:site_name":
				og.SiteName = content
			}
		}
	}
}
