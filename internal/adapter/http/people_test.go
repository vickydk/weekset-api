package httpadapter

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vickydk/weekset-api/internal/adapter/memory"
	peopleuc "github.com/vickydk/weekset-api/internal/usecase/people"
)

func TestAcceptRejectsAnUnknownCode(t *testing.T) {
	e := NewRouter()
	store := memory.NewStore()
	MountPeople(e, &peopleuc.Service{Centers: store, Users: store}, testAuth())

	rec := postJSON(t, e, "/v1/invites/accept", map[string]string{"code": "nope"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":{"code":"validation_error","message":"invite is not valid"}}`, rec.Body.String())
}
