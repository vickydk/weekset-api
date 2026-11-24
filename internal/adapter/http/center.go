package httpadapter

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/platform/apierror"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
	centeruc "github.com/vickydk/weekset-api/internal/usecase/center"
	peopleuc "github.com/vickydk/weekset-api/internal/usecase/people"
)

type centerHandler struct {
	svc    *centeruc.Service
	people *peopleuc.Service
	auth   authHandler
}

// MountCenter adds center registration and the branch routes.
func MountCenter(e *echo.Echo, svc *centeruc.Service, people *peopleuc.Service, authSvc *authuc.Service) {
	h := centerHandler{svc: svc, people: people, auth: authHandler{svc: authSvc}}
	limit := newLimiter(10, time.Minute)
	e.POST("/v1/fitness-centers", h.open, limit.middleware)
	authed := e.Group("/v1", h.auth.requireUser)
	authed.POST("/branches", h.addBranch)
	authed.GET("/branches", h.listBranches)
	authed.PATCH("/fitness-centers", h.updateCenter)
	authed.PATCH("/branches/:id", h.renameBranch)
	authed.POST("/invites", h.inviteManager)
	authed.PATCH("/people/:user_id", h.replaceBranches)
}

type openCenterBody struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	CenterName  string `json:"center_name"`
	BranchName  string `json:"branch_name"`
	Timezone    string `json:"timezone"`
}

func (b openCenterBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.Email, validation.Required, is.Email),
		validation.Field(&b.Password, validation.Required, validation.Length(8, 128)),
		validation.Field(&b.DisplayName, validation.Required, validation.Length(1, 80)),
		validation.Field(&b.CenterName, validation.Required, validation.Length(1, 80)),
		validation.Field(&b.BranchName, validation.Required, validation.Length(1, 80)),
		validation.Field(&b.Timezone, validation.Required),
	)
}

func (h centerHandler) open(c echo.Context) error {
	var body openCenterBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	session, err := h.svc.Open(c.Request().Context(), centeruc.OpenInput{
		Email: body.Email, Password: body.Password, DisplayName: body.DisplayName,
		CenterName: body.CenterName, BranchName: body.BranchName, Timezone: body.Timezone,
	})
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.JSON(http.StatusCreated, sessionJSON(session))
}

type nameBody struct {
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

func (b nameBody) Validate() error { return nil }

func (h centerHandler) addBranch(c echo.Context) error {
	var body nameBody
	if err := c.Bind(&body); err != nil || body.Name == "" {
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", "name cannot be blank")
		return nil
	}
	branch, err := h.svc.AddBranch(c.Request().Context(), userID(c), body.Name)
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]string{"id": branch.ID, "name": branch.Name})
}

func (h centerHandler) listBranches(c echo.Context) error {
	branches, err := h.svc.Branches(c.Request().Context(), userID(c))
	if err != nil {
		return writeCenterError(c, err)
	}
	out := make([]map[string]string, 0, len(branches))
	for _, branch := range branches {
		out = append(out, map[string]string{"id": branch.ID, "name": branch.Name})
	}
	return c.JSON(http.StatusOK, out)
}

func (h centerHandler) updateCenter(c echo.Context) error {
	var body nameBody
	if err := c.Bind(&body); err != nil {
		apierror.Write(c.Response(), http.StatusBadRequest, "invalid_request", "invalid request body")
		return nil
	}
	if err := h.svc.UpdateCenter(c.Request().Context(), userID(c), body.Name, body.Timezone); err != nil {
		return writeCenterError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h centerHandler) renameBranch(c echo.Context) error {
	var body nameBody
	if err := c.Bind(&body); err != nil || body.Name == "" {
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", "name cannot be blank")
		return nil
	}
	err := h.svc.RenameBranch(c.Request().Context(), userID(c), c.Param("id"), body.Name)
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type inviteBody struct {
	Email     string   `json:"email"`
	Role      string   `json:"role"`
	BranchID  string   `json:"branch_id"`
	BranchIDs []string `json:"branch_ids"`
}

func (b inviteBody) Validate() error {
	if err := validation.ValidateStruct(&b,
		validation.Field(&b.Email, validation.Required, is.Email),
		validation.Field(&b.Role, validation.Required, validation.In("manager", "trainer", "client")),
	); err != nil {
		return err
	}
	if b.Role == "manager" {
		return validation.ValidateStruct(&b,
			validation.Field(&b.BranchIDs, validation.Required, validation.Length(1, 20)),
		)
	}
	return validation.ValidateStruct(&b,
		validation.Field(&b.BranchID, validation.Required),
	)
}

func (h centerHandler) inviteManager(c echo.Context) error {
	var body inviteBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	var id, code string
	var expires time.Time
	var err error
	if body.Role == "manager" {
		id, code, expires, err = h.svc.InviteManager(c.Request().Context(), userID(c), body.Email, body.BranchIDs)
	} else {
		id, code, expires, err = h.people.Invite(c.Request().Context(), userID(c), body.Email, body.Role, body.BranchID, requestHeaders(c))
	}
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]string{
		"id": id, "code": code, "expires_at": expires.UTC().Format(time.RFC3339),
	})
}

type branchesBody struct {
	BranchIDs []string `json:"branch_ids"`
}

func (b branchesBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.BranchIDs, validation.Required, validation.Length(1, 20)),
	)
}

func (h centerHandler) replaceBranches(c echo.Context) error {
	var body branchesBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	err := h.svc.ReplaceManagerBranches(c.Request().Context(), userID(c), c.Param("user_id"), body.BranchIDs)
	if err != nil {
		return writeCenterError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func writeCenterError(c echo.Context, err error) error {
	var access *membership.AccessError
	switch {
	case errors.As(err, &access):
		apierror.Write(c.Response(), access.StatusCode(), access.Code, access.Error())
	case errors.Is(err, user.ErrEmailInUse):
		apierror.Write(c.Response(), http.StatusConflict, "conflict", user.ErrEmailInUse.Error())
	case errors.Is(err, user.ErrNotFound):
		apierror.Write(c.Response(), http.StatusNotFound, "not_found", "not found")
	case err.Error() == "manager already belongs to a center" || err.Error() == "account is not waiting as a manager" ||
		errors.Is(err, membership.ErrTrainerHasBranch) || errors.Is(err, membership.ErrAlreadyInCenter) ||
		errors.Is(err, membership.ErrManagerHasCenter) || errors.Is(err, peopleuc.ErrNotWaitingTrainer) ||
		errors.Is(err, peopleuc.ErrNotWaitingClient):
		apierror.Write(c.Response(), http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, peopleuc.ErrInviteInvalid):
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", peopleuc.ErrInviteInvalid.Error())
	default:
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", "check the center details")
	}
	return nil
}
