package apierror

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite_Envelope(t *testing.T) {
	rec := httptest.NewRecorder()

	Write(rec, http.StatusUnauthorized, "unauthorized", "unauthorized")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.JSONEq(t, `{"error":{"code":"unauthorized","message":"unauthorized"}}`, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}
