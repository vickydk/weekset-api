package httpadapter

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/domain/exercise"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
)

// MountExercises lists the twelve seeded movements.
func MountExercises(e *echo.Echo, authSvc *authuc.Service) {
	h := authHandler{svc: authSvc}
	e.GET("/v1/exercises", func(c echo.Context) error {
		rows := make([]map[string]string, 0, len(exercise.Catalog))
		for _, movement := range exercise.Catalog {
			rows = append(rows, map[string]string{"slug": movement.Slug, "name": movement.Name})
		}
		return c.JSON(http.StatusOK, map[string]any{"exercises": rows})
	}, h.requireUser)
}
