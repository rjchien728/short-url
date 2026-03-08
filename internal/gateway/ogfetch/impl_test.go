package ogfetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/rjchien728/short-url/internal/gateway/ogfetch"
)

type OGFetchSuite struct {
	suite.Suite
	fetcher *ogfetch.Fetcher
}

func (s *OGFetchSuite) SetupSuite() {
	s.fetcher = ogfetch.NewFetcher(&http.Client{})
}

// --- Helpers ---

func serveHTML(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
}

// --- Test cases ---

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
	s.Empty(og.Image)
	s.Empty(og.SiteName)
}

func (s *OGFetchSuite) TestFetch_NoOGTags_ReturnsEmpty() {
	const htmlBody = `<!DOCTYPE html>
<html>
<head><title>Plain Page</title></head>
<body><p>No OG tags here.</p></body>
</html>`

	srv := serveHTML(htmlBody)
	defer srv.Close()

	og, err := s.fetcher.Fetch(context.Background(), srv.URL)
	s.Require().NoError(err)
	s.Require().NotNil(og)
	s.Empty(og.Title)
	s.Empty(og.Description)
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

func TestOGFetch(t *testing.T) {
	suite.Run(t, new(OGFetchSuite))
}
