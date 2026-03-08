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
	"github.com/rjchien728/short-url/internal/domain/service"
	"github.com/rjchien728/short-url/internal/handler"
	"github.com/rjchien728/short-url/internal/mock"
)

func setupURLEcho(t *testing.T, svc *mock.MockURLService) *echo.Echo {
	t.Helper()
	e := echo.New()
	handler.RegisterURLRoutes(e, svc, "http://localhost:8080")
	return e
}

func TestURLHandler_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	now := time.Now().UTC().Truncate(time.Second)

	// Happy path — 201 Created
	t.Run("201 created", func(t *testing.T) {
		svc := mock.NewMockURLService(ctrl)
		svc.EXPECT().
			Create(gomock.Any(), service.CreateURLRequest{
				LongURL:   "https://example.com/path",
				CreatorID: "user_01",
			}).
			Return(&entity.ShortURL{
				ShortCode: "Ab3D5fG7hJ",
				LongURL:   "https://example.com/path",
				CreatorID: "user_01",
				CreatedAt: now,
			}, nil)

		e := setupURLEcho(t, svc)
		body := `{"long_url":"https://example.com/path","creator_id":"user_01"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/urls", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "http://localhost:8080/Ab3D5fG7hJ", resp["short_url"])
		assert.Equal(t, "https://example.com/path", resp["long_url"])
		assert.Equal(t, "user_01", resp["creator_id"])
		assert.NotEmpty(t, resp["created_at"])
	})

	// 400 — empty long_url
	t.Run("400 empty long_url", func(t *testing.T) {
		svc := mock.NewMockURLService(ctrl)
		e := setupURLEcho(t, svc)

		body := `{"long_url":"","creator_id":"user_01"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/urls", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "INVALID_ARGUMENT", resp["error"])
	})

	// 400 — invalid scheme (ftp://)
	t.Run("400 invalid scheme", func(t *testing.T) {
		svc := mock.NewMockURLService(ctrl)
		e := setupURLEcho(t, svc)

		body := `{"long_url":"ftp://example.com/file","creator_id":"user_01"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/urls", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "INVALID_ARGUMENT", resp["error"])
	})

	// 400 — URL exceeds 2048 characters
	t.Run("400 url too long", func(t *testing.T) {
		svc := mock.NewMockURLService(ctrl)
		e := setupURLEcho(t, svc)

		longURL := "https://example.com/" + strings.Repeat("a", 2048)
		body := `{"long_url":"` + longURL + `","creator_id":"user_01"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/urls", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "INVALID_ARGUMENT", resp["error"])
	})

	// 400 — empty creator_id
	t.Run("400 empty creator_id", func(t *testing.T) {
		svc := mock.NewMockURLService(ctrl)
		e := setupURLEcho(t, svc)

		body := `{"long_url":"https://example.com","creator_id":""}`
		req := httptest.NewRequest(http.MethodPost, "/v1/urls", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "INVALID_ARGUMENT", resp["error"])
	})

	// 500 — service error
	t.Run("500 service error", func(t *testing.T) {
		svc := mock.NewMockURLService(ctrl)
		svc.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, assert.AnError)

		e := setupURLEcho(t, svc)
		body := `{"long_url":"https://example.com","creator_id":"user_01"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/urls", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "INTERNAL_ERROR", resp["error"])
	})
}
