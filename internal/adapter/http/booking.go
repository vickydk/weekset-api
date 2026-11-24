package httpadapter

import (
	"net/http"
	"time"

	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/platform/apierror"
	"github.com/vickydk/weekset-api/internal/port"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
	scheduleuc "github.com/vickydk/weekset-api/internal/usecase/schedule"
)

type bookingHandler struct {
	svc  *scheduleuc.Service
	auth authHandler
}

// MountBookings adds the request and the caller's week.
func MountBookings(e *echo.Echo, svc *scheduleuc.Service, authSvc *authuc.Service) {
	h := bookingHandler{svc: svc, auth: authHandler{svc: authSvc}}
	authed := e.Group("/v1/bookings", h.auth.requireUser)
	authed.POST("", h.request)
	authed.GET("", h.list)
}

type bookingBody struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

func (b bookingBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.StartsAt, validation.Required),
		validation.Field(&b.EndsAt, validation.Required),
	)
}

func (h bookingHandler) request(c echo.Context) error {
	var body bookingBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	booking, err := h.svc.Request(c.Request().Context(), userID(c), body.StartsAt, body.EndsAt, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.JSON(http.StatusCreated, bookingJSON(booking))
}

func (h bookingHandler) list(c echo.Context) error {
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
	rows, err := h.svc.Bookings(c.Request().Context(), userID(c), from, to, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	out := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, bookingJSON(row))
	}
	return c.JSON(http.StatusOK, map[string]any{"bookings": out})
}

func bookingJSON(booking port.Booking) map[string]string {
	return map[string]string{
		"id":              booking.ID,
		"starts_at":       booking.StartsAt.UTC().Format(time.RFC3339),
		"ends_at":         booking.EndsAt.UTC().Format(time.RFC3339),
		"status":          booking.Status,
		"client_name":     booking.ClientName,
		"trainer_name":    booking.TrainerName,
		"trainer_user_id": booking.TrainerUserID,
		"client_user_id":  booking.ClientUserID,
	}
}
