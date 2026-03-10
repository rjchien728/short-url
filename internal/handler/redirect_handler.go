package handler

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/service"
	"github.com/rjchien728/short-url/internal/pkg/botdetect"
	"github.com/rjchien728/short-url/internal/pkg/logger"
)

// ogHTMLTemplate is the HTML page returned to social bots.
// It contains OG meta tags and a meta-refresh redirect to the original URL.
// og:url is set to the short URL itself so Facebook binds the preview to the
// short URL rather than inferring the canonical from the meta-refresh target.
// og:type is fixed to "website" to avoid requiring additional type-specific tags.
// html/template is used to auto-escape all values and prevent XSS.
var ogHTMLTemplate = template.Must(template.New("og").Parse(`<!DOCTYPE html>
<html>
<head>
<meta property="og:url" content="{{.ShortURL}}" />
<meta property="og:type" content="website" />
<meta property="og:title" content="{{.Title}}" />
<meta property="og:description" content="{{.Description}}" />
<meta property="og:image" content="{{.Image}}" />
<meta property="og:site_name" content="{{.SiteName}}" />
<meta http-equiv="refresh" content="0; url={{.LongURL}}" />
</head>
<body></body>
</html>`))

// RegisterRedirectRoutes mounts the short-code redirect endpoint.
func RegisterRedirectRoutes(e *echo.Echo, svc service.RedirectService) {
	h := &redirectHandler{svc: svc}
	e.GET("/:shortCode", h.redirect)
}

type redirectHandler struct {
	svc service.RedirectService
}

func (h *redirectHandler) redirect(c echo.Context) error {
	ctx := c.Request().Context()
	shortCode := c.Param("shortCode")

	shortURL, err := h.svc.Resolve(ctx, shortCode)
	if err != nil {
		switch {
		case errors.Is(err, entity.ErrNotFound):
			return c.JSON(http.StatusNotFound, errorResponse{
				Error:   "NOT_FOUND",
				Message: "short url not found",
			})
		case errors.Is(err, entity.ErrExpired):
			return c.JSON(http.StatusGone, errorResponse{
				Error:   "URL_EXPIRED",
				Message: "this short url has expired",
			})
		default:
			logger.Error(ctx, "failed to resolve short url", "error", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Error:   "INTERNAL_ERROR",
				Message: "failed to resolve short url",
			})
		}
	}

	userAgent := c.Request().UserAgent()
	isBot := botdetect.IsBot(userAgent)

	// Build and fire-and-forget the click log.
	clickID, _ := uuid.NewV7()
	clickLog := &entity.ClickLog{
		ID:         clickID.String(),
		ShortURLID: shortURL.ID,
		ShortCode:  shortCode,
		CreatorID:  shortURL.CreatorID,
		ReferralID: c.QueryParam("ref"),
		Referrer:   c.Request().Referer(),
		UserAgent:  userAgent,
		IPAddress:  c.RealIP(),
		IsBot:      isBot,
		CreatedAt:  time.Now().UTC(),
	}

	if err := h.svc.RecordClick(ctx, clickLog); err != nil {
		logger.Warn(ctx, "failed to record click event", "error", err)
	}

	// Bot: return OG HTML page.
	// Build the fully-qualified short URL so og:url points to the short link
	// itself rather than the redirect target.
	if isBot {
		scheme := "https"
		if c.Request().TLS == nil {
			scheme = "http"
		}
		shortURLFull := scheme + "://" + c.Request().Host + "/" + shortCode
		html := buildOGHTML(shortURL, shortURLFull)
		return c.HTML(http.StatusOK, html)
	}

	// Regular user: 302 redirect.
	return c.Redirect(http.StatusFound, shortURL.LongURL)
}

// ogTemplateData holds values injected into ogHTMLTemplate.
type ogTemplateData struct {
	Title       string
	Description string
	Image       string
	SiteName    string
	LongURL     string
	ShortURL    string // canonical URL of the short link, used for og:url
}

// buildOGHTML returns an HTML page with OG meta tags for social crawlers.
// shortURL is the fully-qualified short link URL (e.g. https://s.example.com/11xnm)
// and is written into og:url so that Facebook binds the preview to the short
// link rather than to the redirect target.
// All values are escaped by html/template to prevent XSS.
// Falls back to LongURL as title when OGMetadata is missing or fetch failed.
func buildOGHTML(s *entity.ShortURL, shortURL string) string {
	data := ogTemplateData{
		Title:    s.LongURL,
		LongURL:  s.LongURL,
		ShortURL: shortURL,
	}

	if s.OGMetadata != nil && !s.OGMetadata.FetchFailed {
		if s.OGMetadata.Title != "" {
			data.Title = s.OGMetadata.Title
		}
		data.Description = s.OGMetadata.Description
		data.Image = s.OGMetadata.Image
		data.SiteName = s.OGMetadata.SiteName
	}

	var buf bytes.Buffer
	if err := ogHTMLTemplate.Execute(&buf, data); err != nil {
		// Template execution should never fail with a valid struct; fall back to empty page.
		return "<!DOCTYPE html><html><body></body></html>"
	}
	return buf.String()
}
