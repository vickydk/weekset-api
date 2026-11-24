package httpadapter

import (
	"net/http"

	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/port"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
	scheduleuc "github.com/vickydk/weekset-api/internal/usecase/schedule"
)

type soloHandler struct {
	svc  *scheduleuc.Service
	auth authHandler
}

// MountSolo adds the solo-day routes. A solo day is not a booking.
func MountSolo(e *echo.Echo, svc *scheduleuc.Service, authSvc *authuc.Service) {
	h := soloHandler{svc: svc, auth: authHandler{svc: authSvc}}
	authed := e.Group("/v1/solo-plans", h.auth.requireUser)
	authed.GET("/clients", h.clients)
	authed.POST("", h.assign)
	authed.GET("", h.read)
}

type soloBody struct {
	ClientUserID string     `json:"client_user_id"`
	PlanDate     string     `json:"plan_date"`
	Items        []soloItem `json:"items"`
}

type soloItem struct {
	Slug        string `json:"slug"`
	Sets        int    `json:"sets"`
	Reps        int    `json:"reps"`
	RestSeconds int    `json:"rest_seconds"`
}

func (b soloBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.ClientUserID, validation.Required),
		validation.Field(&b.PlanDate, validation.Required),
	)
}

func (h soloHandler) assign(c echo.Context) error {
	var body soloBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	items := make([]port.SoloItem, 0, len(body.Items))
	for _, item := range body.Items {
		items = append(items, port.SoloItem{Slug: item.Slug, Sets: item.Sets, Reps: item.Reps, RestSeconds: item.RestSeconds})
	}
	plan, err := h.svc.AssignSolo(c.Request().Context(), userID(c), body.ClientUserID, body.PlanDate, items, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.JSON(http.StatusCreated, soloJSON(plan))
}

func (h soloHandler) read(c echo.Context) error {
	plan, err := h.svc.Solo(c.Request().Context(), userID(c), c.QueryParam("client_user_id"), c.QueryParam("date"), requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.JSON(http.StatusOK, soloJSON(plan))
}

func (h soloHandler) clients(c echo.Context) error {
	rows, err := h.svc.Clients(c.Request().Context(), userID(c), requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	out := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]string{"user_id": row.UserID, "display_name": row.DisplayName})
	}
	return c.JSON(http.StatusOK, map[string]any{"clients": out})
}

func soloJSON(plan port.SoloPlan) map[string]any {
	items := make([]map[string]any, 0, len(plan.Items))
	for _, item := range plan.Items {
		items = append(items, map[string]any{
			"slug": item.Slug, "sets": item.Sets, "reps": item.Reps, "rest_seconds": item.RestSeconds,
		})
	}
	return map[string]any{"plan_date": plan.PlanDate, "items": items}
}
