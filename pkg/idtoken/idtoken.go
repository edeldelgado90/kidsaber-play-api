// Package idtoken verifies Firebase Authentication ID tokens.
//
// The KidSaber Play clients (web and mobile) sign in anonymously via Firebase
// Auth and send the resulting ID token as `Authorization: Bearer <token>`.
// This package validates that token's signature, issuer, audience and expiry
// against the configured Firebase project.
//
// An ID token is a different artefact from an App Check token (see
// pkg/appcheck): an ID token attests *who* the caller is (an anonymous UID),
// while an App Check token attests *what* the caller is (a genuine app build).
// They are verified by different Firebase APIs and are not interchangeable.
//
// On Cloud Run, authentication uses Application Default Credentials
// automatically. For local development, run `gcloud auth application-default
// login` or set GOOGLE_APPLICATION_CREDENTIALS to a service-account key file.
package idtoken

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
)

// Verifier wraps the Firebase Auth client.
type Verifier struct {
	client *fbauth.Client
}

// NewVerifier initialises a Firebase ID token verifier for projectID.
// projectID must match the Firebase project that issued the tokens.
func NewVerifier(ctx context.Context, projectID string) (*Verifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("initialising firebase app: %w", err)
	}

	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialising firebase auth client: %w", err)
	}

	return &Verifier{client: client}, nil
}

// VerifyIDToken validates a Firebase Authentication ID token.
// Returns nil if the token is valid and non-expired, an error otherwise.
//
// Verification is offline after the first call — the SDK caches Google's
// public keys and refreshes them as they expire, so this does not add a
// network round trip to every request.
func (v *Verifier) VerifyIDToken(ctx context.Context, token string) error {
	_, err := v.client.VerifyIDToken(ctx, token)
	return err
}
