package ogfetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/rjchien728/short-url/internal/gateway/ogfetch"
)

const defaultTestImage = "https://example.com/default.png"

type OGFetchSuite struct {
	suite.Suite
	fetcher *ogfetch.Fetcher
}

func (s *OGFetchSuite) SetupSuite() {
	s.fetcher = ogfetch.NewFetcher(&http.Client{}, defaultTestImage)
}

// --- Helpers ---

func serveHTML(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
}

// --- Test cases: OG tags (primary path) ---

func (s *OGFetchSuite) TestFetch_AllOGTags() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head>
<meta property="og:title" content="My Page Title" />
<meta property="og:description" content="A great description" />
<meta property="og:image" content="https://example.com/img.png" />
<meta property="og:site_name" content="Example Site" />
</head>
<body></body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	s.Require().NotNil(og)
	s.Equal("My Page Title", og.Title)
	s.Equal("A great description", og.Description)
	s.Equal("https://example.com/img.png", og.Image)
	s.Equal("Example Site", og.SiteName)
	s.False(og.FetchFailed)
}

func (s *OGFetchSuite) TestFetch_PartialOGTags_OnlyTitle() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head>
<meta property="og:title" content="Only Title" />
</head>
<body></body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	s.Require().NotNil(og)
	s.Equal("Only Title", og.Title)
	s.Empty(og.Description)
	// No og:image and no fallback image in HTML -> should use defaultImage.
	s.Equal(defaultTestImage, og.Image)
	s.Empty(og.SiteName)
}

// TestFetch_NoOGTags_ReturnsEmpty verifies that a page with no OG tags AND no HTML
// fallback candidates returns empty strings (except image which gets defaultImage).
func (s *OGFetchSuite) TestFetch_NoOGTags_ReturnsEmpty() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head></head>
<body><p>No metadata here.</p></body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	s.Require().NotNil(og)
	s.Empty(og.Title)
	s.Empty(og.Description)
	// No image source anywhere -> falls back to defaultImage.
	s.Equal(defaultTestImage, og.Image)
}

func (s *OGFetchSuite) TestFetch_HTTP404_ReturnsError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().Error(err)
	s.Nil(og)
}

func (s *OGFetchSuite) TestFetch_InvalidURL_ReturnsError() {
	_, err := s.fetcher.Fetch(context.Background(), "not-a-valid-url")
	s.Require().Error(err)
}

// --- Test cases: fallback path ---

// TestFetch_Fallback_TitleFromHTMLTitle verifies that <title> text is used when og:title is absent.
func (s *OGFetchSuite) TestFetch_Fallback_TitleFromHTMLTitle() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head>
  <title>  Fallback Title  </title>
  <meta name="description" content="Some desc" />
</head>
<body></body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	// og:title absent -> should fall back to <title> text (trimmed).
	s.Equal("Fallback Title", og.Title)
}

// TestFetch_Fallback_DescriptionFromMeta verifies that <meta name="description"> is used
// when og:description is absent.
func (s *OGFetchSuite) TestFetch_Fallback_DescriptionFromMeta() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head>
  <meta name="description" content="HTML meta description" />
</head>
<body></body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	// og:description absent -> should fall back to <meta name="description">.
	s.Equal("HTML meta description", og.Description)
}

// TestFetch_Fallback_ImageFromLinkIcon verifies that <link rel="icon"> href is used
// when og:image is absent and takes priority over body <img>.
func (s *OGFetchSuite) TestFetch_Fallback_ImageFromLinkIcon() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head>
  <link rel="icon" href="https://example.com/favicon.ico" />
</head>
<body>
  <img src="https://example.com/hero.jpg" />
</body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	// <link rel="icon"> should win over <img> in body.
	s.Equal("https://example.com/favicon.ico", og.Image)
}

// TestFetch_Fallback_ImageFromBodyImg verifies that when no og:image and no <link rel="icon">
// exist, the first <img src> found in <body> is used.
func (s *OGFetchSuite) TestFetch_Fallback_ImageFromBodyImg() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head></head>
<body>
  <img src="https://example.com/first.jpg" />
  <img src="https://example.com/second.jpg" />
</body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	// Should capture only the first <img>, ignoring subsequent ones.
	s.Equal("https://example.com/first.jpg", og.Image)
}

// TestFetch_Fallback_DefaultImageWhenNoImageFound verifies that the configured default image
// is used when neither og:image, <link rel="icon">, nor any <img> is present.
func (s *OGFetchSuite) TestFetch_Fallback_DefaultImageWhenNoImageFound() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head><title>No Image Page</title></head>
<body><p>Just text.</p></body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	s.Equal(defaultTestImage, og.Image)
}

// TestFetch_OGTitleTakesPriorityOverHTMLTitle ensures OG tags are not overridden by fallbacks.
func (s *OGFetchSuite) TestFetch_OGTitleTakesPriorityOverHTMLTitle() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head>
  <title>HTML Title</title>
  <meta property="og:title" content="OG Title" />
</head>
<body></body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	// og:title must win over <title>.
	s.Equal("OG Title", og.Title)
}

func TestOGFetch(t *testing.T) {
	suite.Run(t, new(OGFetchSuite))
}
