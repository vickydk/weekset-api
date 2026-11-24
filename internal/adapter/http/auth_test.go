package httpadapter

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vickydk/weekset-api/internal/adapter/memory"
	"github.com/vickydk/weekset-api/internal/platform/password"
	"github.com/vickydk/weekset-api/internal/platform/token"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
)

const accessForTest = 15 * 60 * 1000000000

func TestRegisterAndLoginLookTheSameWhenPasswordIsWrong(t *testing.T) {
	e := NewRouter()
	MountAuth(e, testAuth())

	rec := postJSON(t, e, "/v1/auth/register", map[string]string{
		"email":        "maya@harbor.club",
		"password":     "long-enough",
		"display_name": "Maya",
		"account_type": "client",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.NotEmpty(t, created["access_token"])
	assert.Empty(t, created["memberships"])

	unknown := postJSON(t, e, "/v1/auth/login", map[string]string{
		"email": "missing@harbor.club", "password": "long-enough",
	})
	wrong := postJSON(t, e, "/v1/auth/login", map[string]string{
		"email": "maya@harbor.club", "password": "nope-nope",
	})
	assert.Equal(t, http.StatusUnauthorized, unknown.Code)
	assert.Equal(t, http.StatusUnauthorized, wrong.Code)
	assert.JSONEq(t, unknown.Body.String(), wrong.Body.String())
	assert.JSONEq(t, `{"error":{"code":"unauthorized","message":"unauthorized"}}`, unknown.Body.String())
}

func testAuth() *authuc.Service {
	store := memory.NewStore()
	return &authuc.Service{
		Users:  store,
		Tokens: store,
		Resets: store,
		Mailer: store,
		Hasher: password.Hasher{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLen: 16},
		Issuer: token.Issuer{Secret: []byte("test-secret"), TTL: accessForTest},
	}
}

func postJSON(t *testing.T, handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
