package token_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kidsaber/kidsaber-play-api/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSecret = []byte("test-secret-not-for-production")

func TestIssueAndValidate_HappyPath(t *testing.T) {
	deviceID := "550e8400-e29b-41d4-a716-446655440000"
	ttl := 24 * time.Hour

	tok, expiresAt, err := token.Issue(deviceID, ttl, testSecret)
	require.NoError(t, err)
	assert.NotEmpty(t, tok)
	assert.Greater(t, expiresAt, time.Now().Unix())
	assert.InDelta(t, float64(time.Now().Add(ttl).Unix()), float64(expiresAt), 5)

	// Token must contain exactly one dot separator
	assert.Equal(t, 1, strings.Count(tok, "."), "token must have exactly one '.' separator")

	got, err := token.Validate(tok, testSecret)
	require.NoError(t, err)
	assert.Equal(t, deviceID, got)
}

func TestValidate_WrongSecret(t *testing.T) {
	tok, _, err := token.Issue("device-1", time.Hour, testSecret)
	require.NoError(t, err)

	_, err = token.Validate(tok, []byte("wrong-secret"))
	assert.ErrorIs(t, err, token.ErrInvalidToken)
}

func TestValidate_ExpiredToken(t *testing.T) {
	// Issue a token that expired 1 second in the past.
	tok, _, err := token.Issue("device-1", -time.Second, testSecret)
	require.NoError(t, err)

	_, err = token.Validate(tok, testSecret)
	assert.ErrorIs(t, err, token.ErrExpiredToken)
}

func TestValidate_TamperedPayload(t *testing.T) {
	tok, _, err := token.Issue("device-1", time.Hour, testSecret)
	require.NoError(t, err)

	// Flip a character in the payload portion (before the dot)
	dot := strings.IndexByte(tok, '.')
	tampered := "X" + tok[1:dot] + tok[dot:]

	_, err = token.Validate(tampered, testSecret)
	assert.ErrorIs(t, err, token.ErrInvalidToken)
}

func TestValidate_TamperedSignature(t *testing.T) {
	tok, _, err := token.Issue("device-1", time.Hour, testSecret)
	require.NoError(t, err)

	dot := strings.IndexByte(tok, '.')
	tampered := tok[:dot+1] + "AAAABBBBCCCC"

	_, err = token.Validate(tampered, testSecret)
	assert.ErrorIs(t, err, token.ErrInvalidToken)
}

func TestValidate_MissingDot(t *testing.T) {
	_, err := token.Validate("notokenhere", testSecret)
	assert.ErrorIs(t, err, token.ErrInvalidToken)
}

func TestValidate_EmptyString(t *testing.T) {
	_, err := token.Validate("", testSecret)
	assert.ErrorIs(t, err, token.ErrInvalidToken)
}

func TestIssue_EmptyDeviceID(t *testing.T) {
	_, _, err := token.Issue("", time.Hour, testSecret)
	assert.Error(t, err)
}

func TestIssue_DeviceIDWithNewline(t *testing.T) {
	// A deviceID containing \n would break a naive payload split;
	// our implementation uses LastIndexByte so the expiry field is always correct.
	deviceID := "device\nwith\nnewlines"
	tok, _, err := token.Issue(deviceID, time.Hour, testSecret)
	require.NoError(t, err)

	got, err := token.Validate(tok, testSecret)
	require.NoError(t, err)
	assert.Equal(t, deviceID, got)
}

func TestIssue_DifferentSecretsProduceDifferentTokens(t *testing.T) {
	tok1, _, _ := token.Issue("device-1", time.Hour, []byte("secret-a"))
	tok2, _, _ := token.Issue("device-1", time.Hour, []byte("secret-b"))
	assert.NotEqual(t, tok1, tok2)
}
