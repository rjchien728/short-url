package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// RegisterHealthRoutes mounts the health check endpoint.
func RegisterHealthRoutes(e *echo.Echo) {
	e.GET("/healthz", handleHealth)
}

func handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}
