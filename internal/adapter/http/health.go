package httpadapter

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/platform/apierror"
)

// StatusResponse is the body of the probe routes.
type StatusResponse struct {
	Status string `json:"status" example:"ok"`
} // @name StatusResponse

// Health godoc
//
//	@Summary     Liveness
//	@Description Process is up. The Flutter app does not call this. Use it for probes.
//	@Tags        Health
//	@Produce     json
//	@Success     200 {object} StatusResponse
//	@Router      /health [get]
func Health(c echo.Context) error {
	apierror.WriteJSON(c.Response(), http.StatusOK, StatusResponse{Status: "ok"})
	return nil
}

// Ready godoc
//
//	@Summary     Readiness
//	@Description Process is ready to serve. A database check is added when Postgres is wired.
//	@Tags        Health
//	@Produce     json
//	@Success     200 {object} StatusResponse
//	@Router      /ready [get]
func Ready(c echo.Context) error {
	apierror.WriteJSON(c.Response(), http.StatusOK, StatusResponse{Status: "ok"})
	return nil
}
