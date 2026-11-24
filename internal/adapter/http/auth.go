package httpadapter

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/labstack/echo/v4"

	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/platform/apierror"
	"github.com/vickydk/weekset-api/internal/platform/logger"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
)

type authHandler struct {
	svc *authuc.Service
}

type registerBody struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	AccountType string `json:"account_type"`
}

func (b registerBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.Email, validation.Required, is.Email),
		validation.Field(&b.Password, validation.Required, validation.Length(8, 128)),
		validation.Field(&b.DisplayName, validation.Required, validation.Length(1, 80)),
		validation.Field(&b.AccountType, validation.Required, validation.In("client", "trainer", "manager")),
	)
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (b loginBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.Email, validation.Required, is.Email),
		validation.Field(&b.Password, validation.Required),
	)
}

type refreshBody struct {
	RefreshToken string `json:"refresh_token"`
}

func (b refreshBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.RefreshToken, validation.Required),
	)
}

type passwordBody struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (b passwordBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.CurrentPassword, validation.Required),
		validation.Field(&b.NewPassword, validation.Required, validation.Length(8, 128)),
	)
}

type resetBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (b resetBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.Token, validation.Required),
		validation.Field(&b.Password, validation.Required, validation.Length(8, 128)),
	)
}

type emailBody struct {
	Email string `json:"email"`
}

func (b emailBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.Email, validation.Required, is.Email),
	)
}

// MountAuth adds the public auth routes and the signed-in /v1/me routes.
func MountAuth(e *echo.Echo, svc *authuc.Service) {
	h := authHandler{svc: svc}
	limit := newLimiter(10, time.Minute)
	v1 := e.Group("/v1")
	v1.POST("/auth/register", h.register, limit.middleware)
	v1.POST("/auth/login", h.login, limit.middleware)
	v1.POST("/auth/refresh", h.refresh)
	v1.POST("/auth/logout", h.logout)
	v1.POST("/auth/password/forgot", h.forgot, limit.middleware)
	v1.POST("/auth/password/reset", h.reset)
	authed := v1.Group("", h.requireUser)
	authed.GET("/me", h.me)
	authed.POST("/me/password", h.changePassword)
}

func (h authHandler) register(c echo.Context) error {
	var body registerBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	session, err := h.svc.Register(c.Request().Context(), authuc.RegisterInput{
		Email:       body.Email,
		Password:    body.Password,
		DisplayName: body.DisplayName,
		AccountType: user.AccountType(body.AccountType),
	})
	if err != nil {
		return writeUseError(c, err)
	}
	return c.JSON(http.StatusCreated, sessionJSON(session))
}

func (h authHandler) login(c echo.Context) error {
	var body loginBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	session, err := h.svc.Login(c.Request().Context(), body.Email, body.Password)
	if err != nil {
		return writeUseError(c, err)
	}
	return c.JSON(http.StatusOK, sessionJSON(session))
}

func (h authHandler) refresh(c echo.Context) error {
	var body refreshBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	session, err := h.svc.Refresh(c.Request().Context(), body.RefreshToken)
	if err != nil {
		return writeUseError(c, err)
	}
	return c.JSON(http.StatusOK, sessionJSON(session))
}

func (h authHandler) logout(c echo.Context) error {
	var body refreshBody
	if err := c.Bind(&body); err != nil {
		apierror.Write(c.Response(), http.StatusBadRequest, "invalid_request", "invalid request body")
		return nil
	}
	if err := h.svc.Logout(c.Request().Context(), body.RefreshToken); err != nil {
		return writeUseError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h authHandler) forgot(c echo.Context) error {
	var body emailBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	if err := h.svc.Forgot(c.Request().Context(), body.Email); err != nil {
		return writeUseError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h authHandler) reset(c echo.Context) error {
	var body resetBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	if err := h.svc.ResetPassword(c.Request().Context(), body.Token, body.Password); err != nil {
		return writeUseError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h authHandler) me(c echo.Context) error {
	profile, err := h.svc.Me(c.Request().Context(), userID(c))
	if err != nil {
		return writeUseError(c, err)
	}
	return c.JSON(http.StatusOK, profileJSON(profile))
}

func (h authHandler) changePassword(c echo.Context) error {
	var body passwordBody
	if err := decode(c, &body); err != nil {
		return nil
	}
	err := h.svc.ChangePassword(c.Request().Context(), userID(c), body.CurrentPassword, body.NewPassword)
	if err != nil {
		return writeUseError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h authHandler) requireUser(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		header := c.Request().Header.Get("Authorization")
		raw, ok := cutBearer(header)
		if !ok || raw == "" {
			apierror.Write(c.Response(), http.StatusUnauthorized, "unauthorized", "unauthorized")
			return nil
		}
		id, err := h.svc.Subject(raw)
		if err != nil {
			apierror.Write(c.Response(), http.StatusUnauthorized, "unauthorized", "unauthorized")
			return nil
		}
		c.Set("userID", id)
		return next(c)
	}
}

func userID(c echo.Context) string {
	id, _ := c.Get("userID").(string)
	return id
}

func decode(c echo.Context, dest interface{ Validate() error }) error {
	if err := c.Bind(dest); err != nil {
		apierror.Write(c.Response(), http.StatusBadRequest, "invalid_request", "invalid request body")
		return err
	}
	if err := dest.Validate(); err != nil {
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", err.Error())
		return err
	}
	return nil
}

func writeUseError(c echo.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, user.ErrUnauthorized):
		apierror.Write(c.Response(), http.StatusUnauthorized, "unauthorized", "unauthorized")
	case errors.Is(err, user.ErrEmailInUse):
		apierror.Write(c.Response(), http.StatusConflict, "conflict", user.ErrEmailInUse.Error())
	case errors.Is(err, user.ErrResetInvalid):
		apierror.Write(c.Response(), http.StatusBadRequest, "validation_error", user.ErrResetInvalid.Error())
	default:
		logger.Get().Error(c.Request().Context(), "auth failed", logger.Field{Key: "error", Val: err})
		apierror.Write(c.Response(), http.StatusInternalServerError, "internal_error", "unexpected error")
	}
	return nil
}

func cutBearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return "", false
	}
	return header[len(prefix):], true
}

type sessionResponse struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    int             `json:"expires_in"`
	ID           string          `json:"id"`
	Email        string          `json:"email"`
	DisplayName  string          `json:"display_name"`
	AccountType  string          `json:"account_type"`
	Memberships  []membershipDTO `json:"memberships"`
}

type membershipDTO struct {
	Role            string      `json:"role"`
	Status          string      `json:"status"`
	FitnessCenterID string      `json:"fitness_center_id"`
	CenterName      string      `json:"center_name"`
	BranchID        *string     `json:"branch_id"`
	BranchName      *string     `json:"branch_name"`
	Branches        []branchDTO `json:"branches"`
	TrainerUserID   *string     `json:"trainer_user_id"`
}

type branchDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func sessionJSON(session authuc.Session) sessionResponse {
	return sessionResponse{
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
		ExpiresIn:    session.ExpiresIn,
		ID:           session.Profile.ID,
		Email:        session.Profile.Email,
		DisplayName:  session.Profile.DisplayName,
		AccountType:  string(session.Profile.AccountType),
		Memberships:  membershipsJSON(session.Profile),
	}
}

func profileJSON(profile authuc.Profile) meResponse {
	return meResponse{
		ID:          profile.ID,
		Email:       profile.Email,
		DisplayName: profile.DisplayName,
		AccountType: string(profile.AccountType),
		Memberships: membershipsJSON(profile),
	}
}

type meResponse struct {
	ID          string          `json:"id"`
	Email       string          `json:"email"`
	DisplayName string          `json:"display_name"`
	AccountType string          `json:"account_type"`
	Memberships []membershipDTO `json:"memberships"`
}

func membershipsJSON(profile authuc.Profile) []membershipDTO {
	if profile.Memberships == nil {
		return []membershipDTO{}
	}
	out := make([]membershipDTO, 0, len(profile.Memberships))
	for _, row := range profile.Memberships {
		item := membershipDTO{
			Role:            row.Role,
			Status:          row.Status,
			FitnessCenterID: row.FitnessCenterID,
			CenterName:      row.CenterName,
			Branches:        []branchDTO{},
		}
		if row.BranchID != "" {
			item.BranchID = &row.BranchID
			item.BranchName = &row.BranchName
		}
		if row.TrainerUserID != "" {
			item.TrainerUserID = &row.TrainerUserID
		}
		for _, branch := range row.Branches {
			item.Branches = append(item.Branches, branchDTO{ID: branch.ID, Name: branch.Name})
		}
		out = append(out, item)
	}
	return out
}

type limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{limit: limit, window: window, hits: map[string][]time.Time{}}
}

func (l *limiter) middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		key := clientIP(c) + " " + c.Path()
		now := time.Now()
		l.mu.Lock()
		var recent []time.Time
		for _, at := range l.hits[key] {
			if now.Sub(at) < l.window {
				recent = append(recent, at)
			}
		}
		if len(recent) >= l.limit {
			l.hits[key] = recent
			l.mu.Unlock()
			apierror.Write(c.Response(), http.StatusTooManyRequests, "rate_limited", "rate_limited")
			return nil
		}
		l.hits[key] = append(recent, now)
		l.mu.Unlock()
		return next(c)
	}
}

func clientIP(c echo.Context) string {
	host, _, err := net.SplitHostPort(c.Request().RemoteAddr)
	if err != nil {
		return c.RealIP()
	}
	if host == "" {
		return c.RealIP()
	}
	return host
}
