// Package token provides HMAC-SHA256 signed device tokens used by
// POST /auth/token to issue short-lived bearer credentials to mobile clients.
//
// Token format (no external library required):
//
//	<base64url(payload)>.<base64url(HMAC-SHA256(payload, secret))>
//
// where payload = "<deviceID>\n<expiresAtUnixSecs>".
//
// The payload fields are separated by a newline character; the last field is
// always the expiry so deviceIDs containing other printable characters (e.g.
// hyphens in UUIDs) are handled correctly.
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors returned by Validate.
var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// Issue creates a signed device token valid for ttl duration.
// Returns the token string and its expiry as a Unix timestamp in seconds.
func Issue(deviceID string, ttl time.Duration, secret []byte) (token string, expiresAt int64, err error) {
	if deviceID == "" {
		return "", 0, errors.New("deviceID must not be empty")
	}
	expiresAt = time.Now().Add(ttl).Unix()
	payload := encodePayload(deviceID, expiresAt)
	sig := sign([]byte(payload), secret)

	return encode([]byte(payload)) + "." + encode(sig), expiresAt, nil
}

// Validate verifies a token's signature and expiry.
// Returns the deviceID embedded in the token on success.
func Validate(token string, secret []byte) (deviceID string, err error) {
	dot := strings.IndexByte(token, '.')
	if dot < 0 {
		return "", ErrInvalidToken
	}
	rawPayload, rawSig := token[:dot], token[dot+1:]

	payloadBytes, err := decode(rawPayload)
	if err != nil {
		return "", ErrInvalidToken
	}
	sigBytes, err := decode(rawSig)
	if err != nil {
		return "", ErrInvalidToken
	}

	// Constant-time signature verification to prevent timing attacks.
	expected := sign(payloadBytes, secret)
	if !hmac.Equal(sigBytes, expected) {
		return "", ErrInvalidToken
	}

	deviceID, expiresAt, err := decodePayload(string(payloadBytes))
	if err != nil {
		return "", ErrInvalidToken
	}
	if time.Now().Unix() >= expiresAt {
		return "", ErrExpiredToken
	}

	return deviceID, nil
}

// ── internal helpers ─────────────────────────────────────────────────────────

func encodePayload(deviceID string, expiresAt int64) string {
	return fmt.Sprintf("%s\n%d", deviceID, expiresAt)
}

func decodePayload(payload string) (deviceID string, expiresAt int64, err error) {
	// Split on the *last* newline so deviceIDs that contain other printable
	// characters are unambiguously separated from the expiry field.
	idx := strings.LastIndexByte(payload, '\n')
	if idx < 0 {
		return "", 0, errors.New("malformed payload: missing delimiter")
	}
	deviceID = payload[:idx]
	expiresAt, err = strconv.ParseInt(payload[idx+1:], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("malformed payload: bad expiry: %w", err)
	}
	return deviceID, expiresAt, nil
}

func sign(data, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return mac.Sum(nil)
}

// encode returns the standard base64url encoding (no padding) of data.
func encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
