package httpadapter

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/schedule"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/platform/apierror"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
	scheduleuc "github.com/vickydk/weekset-api/internal/usecase/schedule"
)

type availabilityHandler struct {
	svc  *scheduleuc.Service
	auth authHandler
}

// MountAvailability adds the trainer hour routes.
func MountAvailability(e *echo.Echo, svc *scheduleuc.Service, authSvc *authuc.Service) {
	h := availabilityHandler{svc: svc, auth: authHandler{svc: authSvc}}
	authed := e.Group("/v1/availability", h.auth.requireUser)
	authed.PUT("/template", h.replace)
	authed.POST("/exceptions", h.close)
	authed.DELETE("/exceptions/:id", h.reopen)
	authed.GET("", h.hours)
}

type templateBody struct {
	Windows []templateWindow `json:"windows"`
}

type templateWindow struct {
	Weekday    int    `json:"weekday"`
	LocalStart string `json:"local_start"`
	LocalEnd   string `json:"local_end"`
}

func (b templateBody) Validate() error {
	return validation.ValidateStruct(&b, validation.Field(&b.Windows))
}

type exceptionBody struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

func (b exceptionBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.StartsAt, validation.Required),
		validation.Field(&b.EndsAt, validation.Required),
	)
}

func (h availabilityHandler) replace(c echo.Context) error {
	var body templateBody
	if err := decode(c, &body); err != nil {
		return err
	}
	windows := make([]scheduleuc.WindowInput, 0, len(body.Windows))
	for _, window := range body.Windows {
		windows = append(windows, scheduleuc.WindowInput{
			Weekday: window.Weekday, LocalStart: window.LocalStart, LocalEnd: window.LocalEnd,
		})
	}
	err := h.svc.ReplaceTemplate(c.Request().Context(), userID(c), windows, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h availabilityHandler) close(c echo.Context) error {
	var body exceptionBody
	if err := decode(c, &body); err != nil {
		return err
	}
	id, err := h.svc.CloseHour(c.Request().Context(), userID(c), body.StartsAt, body.EndsAt, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]string{"id": id})
}

func (h availabilityHandler) reopen(c echo.Context) error {
	err := h.svc.ReopenHour(c.Request().Context(), userID(c), c.Param("id"), requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h availabilityHandler) hours(c echo.Context) error {
	from, err := time.Parse(time.RFC3339, c.QueryParam("from"))
	if err != nil {
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", "check the hours")
		return nil
	}
	to, err := time.Parse(time.RFC3339, c.QueryParam("to"))
	if err != nil {
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", "check the hours")
		return nil
	}
	hours, err := h.svc.Hours(c.Request().Context(), userID(c), from, to, c.QueryParam("trainer_user_id"), requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	windows := make([]map[string]string, 0, len(hours.Windows))
	for _, window := range hours.Windows {
		windows = append(windows, map[string]string{
			"starts_at": window.Start.UTC().Format(time.RFC3339),
			"ends_at":   window.End.UTC().Format(time.RFC3339),
		})
	}
	body := map[string]any{"windows": windows}
	if hours.Trainer {
		closed := make([]map[string]string, 0, len(hours.Closed))
		for _, window := range hours.Closed {
			closed = append(closed, map[string]string{
				"id":        window.ID,
				"starts_at": window.Start.UTC().Format(time.RFC3339),
				"ends_at":   window.End.UTC().Format(time.RFC3339),
			})
		}
		template := make([]map[string]any, 0, len(hours.Template))
		for _, rule := range hours.Template {
			template = append(template, map[string]any{
				"weekday": rule.Weekday, "local_start": rule.LocalStart, "local_end": rule.LocalEnd,
			})
		}
		body["closed"] = closed
		body["template"] = template
	}
	return c.JSON(http.StatusOK, body)
}

func writeScheduleError(c echo.Context, err error) error {
	var access *membership.AccessError
	switch {
	case errors.As(err, &access):
		apierror.Write(c.Response(), access.StatusCode(), access.Code, access.Error())
	case errors.Is(err, user.ErrUnauthorized):
		apierror.Write(c.Response(), http.StatusUnauthorized, "unauthorized", "unauthorized")
	case errors.Is(err, user.ErrNotFound):
		apierror.Write(c.Response(), http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, schedule.ErrHourBooked):
		apierror.Write(c.Response(), http.StatusConflict, "conflict", schedule.ErrHourBooked.Error())
	default:
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", "check the hours")
	}
	return nil
}
