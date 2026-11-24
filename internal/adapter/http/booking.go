package httpadapter

import (
	"net/http"
	"time"

	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/domain/schedule"
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
	authed.POST("/:id/confirm", h.decide(string(schedule.StatusConfirmed)))
	authed.POST("/:id/decline", h.decide(string(schedule.StatusDeclined)))
	authed.POST("/:id/cancel", h.decide(string(schedule.StatusCancelled)))
	e.GET("/v1/timetable", h.timetable, h.auth.requireUser)
	series := e.Group("/v1/booking-series", h.auth.requireUser)
	series.POST("", h.repeat)
	series.POST("/:id/skip", h.skip)
	series.POST("/:id/move", h.move)
	e.GET("/v1/notifications", h.inbox, h.auth.requireUser)
	e.POST("/v1/notifications/:id/read", h.openNotice, h.auth.requireUser)
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

func (h bookingHandler) decide(status string) echo.HandlerFunc {
	return func(c echo.Context) error {
		booking, err := h.svc.Decide(c.Request().Context(), userID(c), c.Param("id"), status, requestHeaders(c))
		if err != nil {
			return writeScheduleError(c, err)
		}
		return c.JSON(http.StatusOK, bookingJSON(booking))
	}
}

func (h bookingHandler) timetable(c echo.Context) error {
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
	rows, err := h.svc.Timetable(c.Request().Context(), userID(c), from, to, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	out := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		item := bookingJSON(row)
		delete(item, "trainer_user_id")
		delete(item, "client_user_id")
		out = append(out, item)
	}
	return c.JSON(http.StatusOK, map[string]any{"sessions": out})
}

func (h bookingHandler) repeat(c echo.Context) error {
	var body bookingBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	id, err := h.svc.Repeat(c.Request().Context(), userID(c), body.StartsAt, body.EndsAt, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]string{"id": id})
}

type skipBody struct {
	OccurrenceOn string    `json:"occurrence_on"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
}

func (b skipBody) Validate() error {
	return validation.ValidateStruct(&b, validation.Field(&b.OccurrenceOn, validation.Required))
}

func (h bookingHandler) skip(c echo.Context) error {
	var body skipBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	err := h.svc.Skip(c.Request().Context(), userID(c), c.Param("id"), body.OccurrenceOn, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h bookingHandler) move(c echo.Context) error {
	var body skipBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	err := h.svc.Move(c.Request().Context(), userID(c), c.Param("id"), body.OccurrenceOn, body.StartsAt, body.EndsAt, requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h bookingHandler) inbox(c echo.Context) error {
	rows, err := h.svc.Inbox(c.Request().Context(), userID(c), requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"id": row.ID, "kind": row.Kind, "booking_id": row.BookingID,
			"read": row.Read, "created_at": row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"notifications": out})
}

func (h bookingHandler) openNotice(c echo.Context) error {
	notice, err := h.svc.OpenNotice(c.Request().Context(), userID(c), c.Param("id"), requestHeaders(c))
	if err != nil {
		return writeScheduleError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{
		"id": notice.ID, "kind": notice.Kind, "booking_id": notice.BookingID,
	})
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
		"series_id":       booking.SeriesID,
	}
}
