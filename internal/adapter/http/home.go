package httpadapter

import (
	"net/http"

	"github.com/labstack/echo/v4"

	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
	homeuc "github.com/vickydk/weekset-api/internal/usecase/home"
)

// MountHome adds the role dashboard.
func MountHome(e *echo.Echo, svc *homeuc.Service, authSvc *authuc.Service) {
	h := homeHandler{svc: svc, auth: authHandler{svc: authSvc}}
	e.GET("/v1/home", h.home, h.auth.requireUser)
}

type homeHandler struct {
	svc  *homeuc.Service
	auth authHandler
}

func (h homeHandler) home(c echo.Context) error {
	card, err := h.svc.Home(c.Request().Context(), userID(c), requestHeaders(c))
	if err != nil {
		return writeCenterError(c, err)
	}
	body := map[string]any{
		"role":           card.Role,
		"question":       card.Question,
		"center_name":    card.CenterName,
		"branch_name":    card.BranchName,
		"empty_title":    card.EmptyTitle,
		"empty_message":  card.EmptyMessage,
		"primary_action": card.PrimaryAction,
		"attention":      attentionJSON(card.Attention),
		"sessions":       timetableJSON(card.Sessions),
		"branches":       branchJSON(card.Branches),
	}
	if card.Solo != nil {
		body["solo"] = map[string]any{"count": card.Solo.Count, "first": card.Solo.First}
	}
	return c.JSON(http.StatusOK, body)
}

func attentionJSON(rows []homeuc.Attention) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]string{"label": row.Label, "action": row.Action})
	}
	return out
}

func timetableJSON(rows []homeuc.Session) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]string{
			"id": row.ID, "time": row.Time, "client_name": row.ClientName, "trainer_name": row.TrainerName,
			"branch_name": row.BranchName, "status": row.Status,
		})
	}
	return out
}

func branchJSON(rows []homeuc.BranchLoad) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"name": row.Name, "sessions_today": row.SessionsToday,
			"requests_waiting": row.RequestsWaiting, "clients_without_pt": row.ClientsWithoutPT,
		})
	}
	return out
}
