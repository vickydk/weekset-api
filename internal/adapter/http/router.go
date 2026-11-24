package httpadapter

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/vickydk/weekset-api/internal/platform/logger"
)

// NewRouter is the HTTP surface for this scaffold: liveness and readiness.
func NewRouter() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Logger.SetOutput(io.Discard)

	e.Use(middleware.Recover())
	e.Use(accessLog())

	e.GET("/health", Health)
	e.GET("/ready", Ready)
	return e
}

func accessLog() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			id := newRequestID()
			ctx := logger.WithRequestID(c.Request().Context(), id)
			c.SetRequest(c.Request().WithContext(ctx))

			err := next(c)

			logger.Get().Info(ctx, "request",
				logger.Field{Key: "method", Val: c.Request().Method},
				logger.Field{Key: "path", Val: c.Path()},
				logger.Field{Key: "status", Val: c.Response().Status},
				logger.Field{Key: "duration_ms", Val: time.Since(start).Milliseconds()},
			)
			return err
		}
	}
}

func newRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buf[:])
}
