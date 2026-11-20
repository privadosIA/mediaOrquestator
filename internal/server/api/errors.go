// Package api is the kernel shared by the families of internal/server: the JSON and error helpers, the error
// codes, the route-registration signature and the small patterns more than one family checks. It knows nothing
// of the server and imports no other server package.
package api

import (
	"encoding/json"
	"net/http"
)

// HandleFunc is how a family registers an endpoint: the pattern ("GET /api/x/{id}") and its handler. The server
// wraps each one with the gate (auth, content mode, CORS).
type HandleFunc func(pattern string, h http.HandlerFunc)

// APIError is the JSON body of every error response: a machine-readable
// code plus a human-readable message (see specs/http-server).
type APIError struct {
	Error APIErrorBody `json:"error"`
}

// APIErrorBody is the "error" object of APIError.
type APIErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Provider and Alternatives are set by describe_refused: who refused, and what the
	// user can retry with (a model at a provider each).
	Provider     string `json:"provider,omitempty"`
	Alternatives any    `json:"alternatives,omitempty"`
	// Reason is the availability reason of the engine behind the error (engine_not_running, no_gpu...).
	Reason string `json:"reason,omitempty"`
}

// Error codes used across handlers.
const (
	CodeInvalid           = "invalid_request"
	CodeValidation        = "validation_failed"
	CodeNotFound          = "not_found"
	CodeConflict          = "conflict"
	CodeForbidden         = "forbidden"
	CodeEngineNotRunning  = "engine_not_running"
	CodeContentNotAllowed = "content_not_allowed"
	CodeInternal          = "internal_error"
	CodeMethodNotAllowed  = "method_not_allowed"
	CodeConsentRequired   = "consent_required"
	CodeNoModel           = "no_model"
	CodeModelNotAllowed   = "model_not_allowed"
	CodeTooManyCalls      = "too_many_calls"
	CodeQueueFull         = "queue_full"
	CodeTooManyAttempts   = "too_many_attempts"
	// CodeUnavailable marks a failure of something the request depends on (Ollama, the model, memory) rather
	// than of the request itself.
	CodeUnavailable = "unavailable"
	// CodeAssistRefused marks a model that declined to write the text.
	CodeAssistRefused = "assist_refused"
)

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the structured error body with the given status.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, APIError{Error: APIErrorBody{Code: code, Message: message}})
}
