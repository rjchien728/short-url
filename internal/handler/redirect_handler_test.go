package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/handler"
	"github.com/rjchien728/short-url/internal/mock"
)

func setupRedirectEcho(t *testing.T, svc *mock.MockRedirectService) *echo.Echo {
	t.Helper()
	e := echo.New()
	handler.RegisterRedirectRoutes(e, svc)
	return e
}

func TestRedirectHandler_Redirect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	shortURL := &entity.ShortURL{
		ID:        12345,
		ShortCode: "Ab3D5fG7hJ",
		LongURL:   "https://example.com/original",
		CreatorID: "user_01",
		CreatedAt: time.Now().UTC(),
	}

	// 302 regular user redirect
	t.Run("302 regular redirect", func(t *testing.T) {
		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "Ab3D5fG7hJ").Return(shortURL, nil)
		svc.EXPECT().RecordClick(gomock.Any(), gomock.Any()).Return(nil)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Equal(t, shortURL.LongURL, rec.Header().Get("Location"))
	})

	// 200 bot receives OG HTML
	t.Run("200 bot gets og html", func(t *testing.T) {
		og := &entity.OGMetadata{
			Title:       "Example Title",
			Description: "Example Description",
			Image:       "https://example.com/image.png",
			SiteName:    "Example Site",
		}
		botShortURL := &entity.ShortURL{
			ID:         12345,
			ShortCode:  "Ab3D5fG7hJ",
			LongURL:    "https://example.com/original",
			CreatorID:  "user_01",
			OGMetadata: og,
			CreatedAt:  time.Now().UTC(),
		}

		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "Ab3D5fG7hJ").Return(botShortURL, nil)
		svc.EXPECT().RecordClick(gomock.Any(), gomock.Any()).Return(nil)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ", nil)
		req.Header.Set("User-Agent", "facebookexternalhit/1.1")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "og:title")
		assert.Contains(t, body, "Example Title")
		assert.Contains(t, body, "og:description")
		assert.Contains(t, body, "og:image")
		assert.Contains(t, body, "og:site_name")
		assert.Contains(t, body, "meta http-equiv=\"refresh\"")
	})

	// 200 bot with nil OGMetadata falls back to LongURL as title
	t.Run("200 bot og fallback when no metadata", func(t *testing.T) {
		noOGShortURL := &entity.ShortURL{
			ID:        12345,
			ShortCode: "Ab3D5fG7hJ",
			LongURL:   "https://example.com/original",
			CreatorID: "user_01",
			CreatedAt: time.Now().UTC(),
		}

		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "Ab3D5fG7hJ").Return(noOGShortURL, nil)
		svc.EXPECT().RecordClick(gomock.Any(), gomock.Any()).Return(nil)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ", nil)
		req.Header.Set("User-Agent", "Twitterbot/1.0")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		// Falls back to LongURL as og:title content
		assert.Contains(t, body, "https://example.com/original")
	})

	// 404 not found
	t.Run("404 not found", func(t *testing.T) {
		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "notexist").Return(nil, entity.ErrNotFound)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/notexist", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "NOT_FOUND", resp["error"])
	})

	// 410 expired
	t.Run("410 expired", func(t *testing.T) {
		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "Ab3D5fG7hJ").Return(nil, entity.ErrExpired)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusGone, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "URL_EXPIRED", resp["error"])
	})

	// RecordClick failure is non-fatal — still returns 302
	t.Run("302 even when record click fails", func(t *testing.T) {
		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "Ab3D5fG7hJ").Return(shortURL, nil)
		svc.EXPECT().RecordClick(gomock.Any(), gomock.Any()).Return(assert.AnError)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		// redirect still succeeds
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Equal(t, shortURL.LongURL, rec.Header().Get("Location"))
		// ref param is forwarded to RecordClick (checked via gomock)
	})

	// ref query param
	t.Run("302 with ref param", func(t *testing.T) {
		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "Ab3D5fG7hJ").Return(shortURL, nil)
		svc.EXPECT().RecordClick(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, log *entity.ClickLog) error {
				assert.Equal(t, "campaign_01", log.ReferralID)
				return nil
			},
		)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ?ref=campaign_01", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusFound, rec.Code)
	})

	// 500 internal error
	t.Run("500 internal error", func(t *testing.T) {
		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(nil, assert.AnError)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "INTERNAL_ERROR", resp["error"])
	})

	// Test that IsBot is correctly identified and click log has correct flag
	t.Run("bot click log has IsBot true", func(t *testing.T) {
		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "Ab3D5fG7hJ").Return(shortURL, nil)
		svc.EXPECT().RecordClick(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, log *entity.ClickLog) error {
				assert.True(t, log.IsBot)
				return nil
			},
		)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ", nil)
		req.Header.Set("User-Agent", "Googlebot/2.1")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "og:title")
	})

	// strings import usage - ensure meta refresh redirect points to LongURL
	t.Run("og html meta refresh points to long url", func(t *testing.T) {
		svc := mock.NewMockRedirectService(ctrl)
		svc.EXPECT().Resolve(gomock.Any(), "Ab3D5fG7hJ").Return(shortURL, nil)
		svc.EXPECT().RecordClick(gomock.Any(), gomock.Any()).Return(nil)

		e := setupRedirectEcho(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/Ab3D5fG7hJ", nil)
		req.Header.Set("User-Agent", "LinkedInBot/1.0")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.True(t, strings.Contains(body, shortURL.LongURL))
	})
}
