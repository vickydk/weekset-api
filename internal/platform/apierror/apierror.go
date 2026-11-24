// Package apierror is the only place that writes the error JSON envelope.
package apierror

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the JSON body for every failed request.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
} // @name ErrorResponse

// ErrorDetail is the code the app branches on, plus a safe message.
type ErrorDetail struct {
	Code    string `json:"code" example:"unauthorized"`
	Message string `json:"message" example:"unauthorized"`
} // @name ErrorDetail

// Write sends a failure envelope. message must be safe for the client.
func Write(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{Error: ErrorDetail{Code: code, Message: message}})
}

// WriteJSON sends a success body with no extra wrapper.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
