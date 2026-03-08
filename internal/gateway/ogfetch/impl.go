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
	client       *http.Client
	defaultImage string // used as final fallback when no image is found in the HTML
}

// NewFetcher creates a new OG metadata fetcher with the given HTTP client.
// defaultImage is the URL returned as og.Image when neither og:image, <link rel="icon">,
// nor a body <img> is present.
func NewFetcher(client *http.Client, defaultImage string) *Fetcher {
	return &Fetcher{client: client, defaultImage: defaultImage}
}

// Fetch retrieves the Open Graph metadata from the target URL by parsing its HTML.
// Priority order for each field:
//
//	Title:       og:title  >  <title> text
//	Description: og:description  >  <meta name="description">
//	Image:       og:image  >  <link rel="icon">  >  first <img src> in <body>  >  defaultImage
//	SiteName:    og:site_name only
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
	if err := parseOGTags(resp, og, f.defaultImage); err != nil {
		return nil, fmt.Errorf("ogfetch.Fetch: parse html: %w", err)
	}
	return og, nil
}

// parseOGTags tokenizes the HTML response and extracts metadata with fallback support.
//
// Scan strategy:
//   - <head>: collect OG meta tags, <title> text, <meta name="description">, <link rel="icon">
//   - <body>: collect only the first <img src> then stop immediately
//   - ErrorToken (EOF): merge fallbacks and return
func parseOGTags(resp *http.Response, og *entity.OGMetadata, defaultImage string) error {
	tokenizer := html.NewTokenizer(resp.Body)

	// fallback candidates — only applied when the OG field is empty after scanning.
	var (
		fallbackTitle       string
		fallbackDescription string
		fallbackImage       string // set from <link rel="icon">
		captureTitle        bool   // true when we are inside a <title> tag
		inBody              bool
	)

	for {
		tt := tokenizer.Next()

		switch tt {
		case html.ErrorToken:
			// End of document (or read error) — apply fallbacks and return.
			applyFallbacks(og, fallbackTitle, fallbackDescription, fallbackImage, defaultImage)
			return nil

		case html.TextToken:
			// Capture <title>...</title> text for the title fallback.
			if captureTitle {
				text := strings.TrimSpace(string(tokenizer.Text()))
				if text != "" {
					fallbackTitle = text
				}
			}

		case html.EndTagToken:
			token := tokenizer.Token()
			if token.Data == "title" {
				captureTitle = false
			}

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()

			switch token.Data {
			case "body":
				// Entering <body>: OG tags and <title> are now behind us.
				// If we already have all fallbacks we need, stop early.
				inBody = true
				captureTitle = false
				if fallbackImage != "" || og.Image != "" {
					// No need to scan body for images.
					applyFallbacks(og, fallbackTitle, fallbackDescription, fallbackImage, defaultImage)
					return nil
				}

			case "title":
				if !inBody {
					captureTitle = true
				}

			case "meta":
				parseMeta(token, og, &fallbackDescription)

			case "link":
				if !inBody && fallbackImage == "" {
					if href := parseLinkIcon(token); href != "" {
						fallbackImage = href
					}
				}

			case "img":
				// Only look for <img> inside <body>, and stop after the first one.
				if inBody && fallbackImage == "" && og.Image == "" {
					if src := attrVal(token.Attr, "src"); src != "" {
						fallbackImage = src
						applyFallbacks(og, fallbackTitle, fallbackDescription, fallbackImage, defaultImage)
						return nil
					}
				}
			}
		}
	}
}

// parseMeta handles <meta> tags: extracts OG properties and the description fallback.
func parseMeta(token html.Token, og *entity.OGMetadata, fallbackDescription *string) {
	var property, name, content string
	for _, attr := range token.Attr {
		switch strings.ToLower(attr.Key) {
		case "property":
			property = strings.ToLower(attr.Val)
		case "name":
			name = strings.ToLower(attr.Val)
		case "content":
			content = attr.Val
		}
	}

	// OG properties take highest priority.
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

	// <meta name="description"> provides the description fallback.
	if name == "description" && *fallbackDescription == "" {
		*fallbackDescription = content
	}
}

// parseLinkIcon returns the href of a <link rel="icon"> or <link rel="shortcut icon"> tag.
func parseLinkIcon(token html.Token) string {
	rel := strings.ToLower(attrVal(token.Attr, "rel"))
	if rel == "icon" || rel == "shortcut icon" {
		return attrVal(token.Attr, "href")
	}
	return ""
}

// applyFallbacks fills empty OG fields using the collected fallback values.
func applyFallbacks(og *entity.OGMetadata, title, description, image, defaultImage string) {
	if og.Title == "" {
		og.Title = title
	}
	if og.Description == "" {
		og.Description = description
	}
	if og.Image == "" {
		if image != "" {
			og.Image = image
		} else {
			og.Image = defaultImage
		}
	}
}

// attrVal returns the value of the first attribute matching the given key (case-insensitive).
func attrVal(attrs []html.Attribute, key string) string {
	for _, a := range attrs {
		if strings.ToLower(a.Key) == key {
			return a.Val
		}
	}
	return ""
}
