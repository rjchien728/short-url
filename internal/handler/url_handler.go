package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/rjchien728/short-url/internal/domain/service"
	"github.com/rjchien728/short-url/internal/pkg/logger"
)

// createURLRequest is the JSON body for POST /v1/urls.
type createURLRequest struct {
	LongURL   string `json:"long_url"`
	CreatorID string `json:"creator_id"`
}

// createURLResponse is the JSON body returned on 201.
type createURLResponse struct {
	ShortURL  string `json:"short_url"`
	LongURL   string `json:"long_url"`
	CreatorID string `json:"creator_id"`
	CreatedAt string `json:"created_at"`
}

// RegisterURLRoutes mounts the URL creation endpoint.
// baseURL is used to construct the full short URL in the response (e.g. "http://localhost:8080").
func RegisterURLRoutes(e *echo.Echo, svc service.URLService, baseURL string) {
	h := &urlHandler{svc: svc, baseURL: strings.TrimRight(baseURL, "/")}
	e.POST("/v1/urls", h.create)
}

type urlHandler struct {
	svc     service.URLService
	baseURL string
}

func (h *urlHandler) create(c echo.Context) error {
	ctx := c.Request().Context()

	var req createURLRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "INVALID_ARGUMENT",
			Message: "invalid request body",
		})
	}

	// Validate long_url
	if msg, ok := validateLongURL(req.LongURL); !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "INVALID_ARGUMENT",
			Message: msg,
		})
	}

	// Validate creator_id
	if strings.TrimSpace(req.CreatorID) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "INVALID_ARGUMENT",
			Message: "creator_id is required",
		})
	}

	result, err := h.svc.Create(ctx, service.CreateURLRequest{
		LongURL:   req.LongURL,
		CreatorID: req.CreatorID,
	})
	if err != nil {
		logger.Error(ctx, "failed to create short url", "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "failed to create short url",
		})
	}

	return c.JSON(http.StatusCreated, createURLResponse{
		ShortURL:  h.baseURL + "/" + result.ShortCode,
		LongURL:   result.LongURL,
		CreatorID: result.CreatorID,
		CreatedAt: result.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// validateLongURL checks the long_url field against the API spec rules.
// Returns (message, false) on validation failure.
func validateLongURL(longURL string) (string, bool) {
	if strings.TrimSpace(longURL) == "" {
		return "long_url is required", false
	}

	if len(longURL) > 2048 {
		return "long_url must be at most 2048 characters", false
	}

	parsed, err := url.ParseRequestURI(longURL)
	if err != nil {
		return "long_url is not a valid URL", false
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "long_url scheme must be http or https", false
	}

	return "", true
}
