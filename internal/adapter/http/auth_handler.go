package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/kidsaber/kidsaber-play-api/pkg/token"
)

// TokenValidator can validate a bearer credential issued by POST /auth/token.
// Implemented by TokenService; can be swapped in tests.
type TokenValidator interface {
	ValidateToken(tok string) error
}

// TokenService issues and validates HMAC-SHA256 signed device tokens.
// It wraps pkg/token and holds the signing secret and TTL.
type TokenService struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenService creates a TokenService with the given signing secret and TTL.
func NewTokenService(secret []byte, ttl time.Duration) *TokenService {
	return &TokenService{secret: secret, ttl: ttl}
}

// ValidateToken implements TokenValidator.
// Returns nil if tok is a valid, non-expired device token signed with the service secret.
func (s *TokenService) ValidateToken(tok string) error {
	_, err := token.Validate(tok, s.secret)
	return err
}

// TokenHandler handles POST /auth/token.
type TokenHandler struct {
	service *TokenService
	logger  *slog.Logger
}

// NewTokenHandler creates a TokenHandler.
func NewTokenHandler(service *TokenService, logger *slog.Logger) *TokenHandler {
	return &TokenHandler{service: service, logger: logger}
}

// Issue handles POST /auth/token.
//
//	Request body:  { "deviceId": "<uuid>" }
//	Response body: { "token": "<signed-token>", "expiresAt": <unix-timestamp> }
//
// The returned token must be sent on subsequent requests as:
//
//	Authorization: Bearer <token>
//
// The endpoint is public (no prior auth required) and is IP-rate-limited by the
// global middleware. The deviceId is not a secret — it is a stable device
// identifier used only for per-device token issuance. No deviceId data is
// stored server-side.
func (h *TokenHandler) Issue(w http.ResponseWriter, r *http.Request) {
	var req TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be valid JSON")
		return
	}

	if req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "deviceId is required")
		return
	}
	if !isValidDeviceID(req.DeviceID) {
		writeError(w, http.StatusBadRequest, "invalid_request", "deviceId must be printable ASCII, max 128 characters")
		return
	}

	tok, expiresAt, err := token.Issue(req.DeviceID, h.service.ttl, h.service.secret)
	if err != nil {
		h.logger.Error("failed to issue device token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, TokenResponse{Token: tok, ExpiresAt: expiresAt})
}

// isValidDeviceID rejects device IDs that would produce malformed token payloads.
// Allows printable ASCII only (32–126), max 128 characters. UUIDs, short
// alphanumeric strings, and most common identifiers pass this check.
func isValidDeviceID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		// Allow all printable ASCII (space through tilde).
		// Newlines are excluded because the payload uses "\n" as a delimiter.
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
