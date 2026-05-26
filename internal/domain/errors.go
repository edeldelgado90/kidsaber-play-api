package domain

import "errors"

// Sentinel domain errors returned by use cases and adapters.
var (
	// ErrNoValidQuestions is returned when LLM generation fails after all retries.
	ErrNoValidQuestions = errors.New("no valid questions could be generated")

	// ErrRateLimit is returned when the LLM provider's rate limit is exceeded.
	ErrRateLimit = errors.New("LLM provider rate limit exceeded")

	// ErrLLMTimeout is returned when the LLM call exceeds the configured timeout.
	ErrLLMTimeout = errors.New("LLM call timed out")

	// ErrInvalidParams is returned for invalid or missing request parameters.
	ErrInvalidParams = errors.New("invalid request parameters")

	// ErrNotFound is returned when no questions are found in the pool.
	ErrNotFound = errors.New("questions not found in pool")
)
