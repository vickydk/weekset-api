package httpadapter

import (
	"net/http"
	"time"

	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
	peopleuc "github.com/vickydk/weekset-api/internal/usecase/people"
)

// MountPeople adds the roster, the code accept, and the trainer assignment.
func MountPeople(e *echo.Echo, svc *peopleuc.Service, authSvc *authuc.Service) {
	h := centerHandler{people: svc, auth: authHandler{svc: authSvc}}
	limit := newLimiter(10, time.Minute)
	e.POST("/v1/invites/accept", h.acceptInvite, limit.middleware)
	authed := e.Group("/v1", h.auth.requireUser)
	authed.POST("/assignments", h.assignTrainer)
	authed.GET("/people", h.listPeople)
	authed.POST("/people/:user_id/suspend", h.suspendPerson)
	authed.POST("/people/:user_id/restore", h.restorePerson)
	authed.DELETE("/people/:user_id", h.removePerson)
}

type codeBody struct {
	Code string `json:"code"`
}

func (b codeBody) Validate() error {
	return validation.ValidateStruct(&b, validation.Field(&b.Code, validation.Required))
}

func (h centerHandler) acceptInvite(c echo.Context) error {
	var body codeBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	if err := h.people.Accept(c.Request().Context(), body.Code); err != nil {
		return writeCenterError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type assignBody struct {
	ClientUserID  string `json:"client_user_id"`
	TrainerUserID string `json:"trainer_user_id"`
}

func (b assignBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.ClientUserID, validation.Required),
		validation.Field(&b.TrainerUserID, validation.Required),
	)
}

func (h centerHandler) assignTrainer(c echo.Context) error {
	var body assignBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	err := h.people.Assign(c.Request().Context(), userID(c), body.ClientUserID, body.TrainerUserID, requestHeaders(c))
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h centerHandler) listPeople(c echo.Context) error {
	page, err := h.people.People(c.Request().Context(), userID(c), requestHeaders(c))
	if err != nil {
		return writeCenterError(c, err)
	}
	people := make([]map[string]string, 0, len(page.People))
	for _, person := range page.People {
		people = append(people, map[string]string{
			"user_id": person.UserID, "display_name": person.DisplayName, "email": person.Email,
			"role": person.Role, "status": person.Status, "branch_name": person.BranchName,
			"trainer_name": person.TrainerName,
		})
	}
	invites := make([]map[string]string, 0, len(page.Invites))
	for _, invite := range page.Invites {
		invites = append(invites, map[string]string{
			"email": invite.Email, "role": invite.Role, "branch_name": invite.BranchName, "expires_at": invite.ExpiresAt,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"people": people, "invites": invites})
}

func (h centerHandler) suspendPerson(c echo.Context) error {
	err := h.people.Suspend(c.Request().Context(), userID(c), c.Param("user_id"), requestHeaders(c))
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h centerHandler) restorePerson(c echo.Context) error {
	err := h.people.Restore(c.Request().Context(), userID(c), c.Param("user_id"), requestHeaders(c))
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h centerHandler) removePerson(c echo.Context) error {
	err := h.people.Remove(c.Request().Context(), userID(c), c.Param("user_id"), requestHeaders(c))
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func requestHeaders(c echo.Context) membership.Headers {
	return membership.Headers{
		CenterID: c.Request().Header.Get("X-Weekset-Center"),
		BranchID: c.Request().Header.Get("X-Weekset-Branch"),
	}
}
