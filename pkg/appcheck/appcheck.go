// Package appcheck provides Firebase App Check token validation.
// It wraps the Firebase Admin SDK to verify that requests originate from
// genuine instances of the registered iOS, Android, or web app.
//
// On Cloud Run, authentication uses Application Default Credentials
// automatically — no extra configuration is needed.
// For local development, run `gcloud auth application-default login`
// or set GOOGLE_APPLICATION_CREDENTIALS to a service-account key file.
// When FIREBASE_PROJECT_ID is not set, the validator is not initialised and
// only static API key auth is accepted.
package appcheck

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	fbappcheck "firebase.google.com/go/v4/appcheck"
)

// Validator wraps the Firebase App Check client.
type Validator struct {
	client *fbappcheck.Client
}

// NewValidator initialises a Firebase App Check validator for projectID.
// projectID must match the Firebase / Google Cloud project that issued the tokens.
func NewValidator(ctx context.Context, projectID string) (*Validator, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("initialising firebase app: %w", err)
	}

	client, err := app.AppCheck(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialising app check client: %w", err)
	}

	return &Validator{client: client}, nil
}

// VerifyToken validates a Firebase App Check token.
// Returns nil if the token is valid and non-expired, an error otherwise.
func (v *Validator) VerifyToken(token string) error {
	_, err := v.client.VerifyToken(token)
	return err
}
