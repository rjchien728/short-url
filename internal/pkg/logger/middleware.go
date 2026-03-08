package logger

import (
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Middleware returns an echo middleware that sets up request-scoped logging.
// It injects a request_id and a pre-configured slog.Logger into the request context,
// then logs the request start and completion with status code and duration.
func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			requestID := uuid.New().String()

			// Create a logger with request metadata
			reqLogger := slog.Default().With(
				"request_id", requestID,
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
			)

			// Inject logger into request context
			ctx := WithLogger(c.Request().Context(), reqLogger)
			c.SetRequest(c.Request().WithContext(ctx))

			Info(ctx, "request started")

			err := next(c)

			Info(ctx, "request completed",
				"status", c.Response().Status,
				"duration_ms", time.Since(start).Milliseconds(),
			)

			return err
		}
	}
}
