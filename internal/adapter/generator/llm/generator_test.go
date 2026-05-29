package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/llm"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
	"github.com/kidsaber/kidsaber-play-api/pkg/validator"
)

// ─── fixtures ─────────────────────────────────────────────────────────────────

// validOptionMultipleJSON is a well-formed LLM response for option_multiple, mathematics, grade 3.
const validOptionMultipleJSON = `[{
	"type": "option_multiple",
	"subject": "mathematics",
	"grade": 3,
	"topic": "multiplication",
	"statement": "¿Cuánto es 4 × 6?",
	"options": [
		{"id": "A", "text": "20"},
		{"id": "B", "text": "24"},
		{"id": "C", "text": "26"},
		{"id": "D", "text": "16"}
	],
	"correctAnswers": ["B"],
	"meta": {"difficulty": "easy", "timeLimitMs": 15000, "tags": ["multiplication"]}
}]`

// twoQuestionsJSON contains two valid option_multiple questions.
const twoQuestionsJSON = `[
	{
		"type": "option_multiple", "subject": "mathematics", "grade": 3,
		"topic": "multiplication", "statement": "¿Cuánto es 3 × 4?",
		"options": [{"id":"A","text":"10"},{"id":"B","text":"12"},{"id":"C","text":"9"},{"id":"D","text":"15"}],
		"correctAnswers": ["B"],
		"meta": {"difficulty": "easy", "timeLimitMs": 15000, "tags": []}
	},
	{
		"type": "option_multiple", "subject": "mathematics", "grade": 3,
		"topic": "multiplication", "statement": "¿Cuánto es 5 × 6?",
		"options": [{"id":"A","text":"25"},{"id":"B","text":"30"},{"id":"C","text":"35"},{"id":"D","text":"20"}],
		"correctAnswers": ["B"],
		"meta": {"difficulty": "medium", "timeLimitMs": 15000, "tags": []}
	}
]`

// wrongFieldsJSON is valid against the schema but has a different subject and grade than the params,
// allowing tests to verify that the generator overwrites them with values from params.
const wrongFieldsJSON = `[{
	"type": "option_multiple",
	"subject": "language",
	"grade": 5,
	"topic": "multiplication",
	"statement": "¿Cuánto es 4 × 6?",
	"options": [
		{"id": "A", "text": "20"},
		{"id": "B", "text": "24"},
		{"id": "C", "text": "26"},
		{"id": "D", "text": "16"}
	],
	"correctAnswers": ["B"],
	"meta": {"difficulty": "easy", "timeLimitMs": 15000, "tags": []}
}]`

// invalidSchemaJSON parses as JSON but fails schema validation (missing required fields).
const invalidSchemaJSON = `[{"type": "option_multiple", "subject": "mathematics", "grade": 3}]`

var defaultParams = questions.GenerateParams{
	Subject: domain.SubjectMathematics,
	Grade:   3,
	Type:    domain.GameTypeOptionMultiple,
	Topic:   "multiplication",
	Count:   1,
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// openAIResp wraps content in the OpenAI-compatible chat completions response envelope.
func openAIResp(content string) []byte {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type choice struct {
		Message msg `json:"message"`
	}
	type resp struct {
		Choices []choice `json:"choices"`
	}
	b, _ := json.Marshal(resp{Choices: []choice{{Message: msg{Role: "assistant", Content: content}}}})
	return b
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newGenerator wires up an LLMGenerator backed by a throwaway httptest.Server.
func newGenerator(t *testing.T, handler http.HandlerFunc, maxRetries int, retryDelay time.Duration) *llm.LLMGenerator {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := llm.NewLLMClient(srv.URL+"/", "", "test-model", 5*time.Second)
	v, err := validator.NewQuestionValidator()
	require.NoError(t, err)
	picker := llm.NewTopicPicker()
	return llm.NewLLMGenerator(client, v, picker, maxRetries, retryDelay, testLogger())
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestLLMGenerator_Generate_SuccessOnFirstAttempt(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openAIResp(validOptionMultipleJSON))
	}
	gen := newGenerator(t, handler, 2, time.Millisecond)

	qs, err := gen.Generate(context.Background(), defaultParams)

	require.NoError(t, err)
	require.Len(t, qs, 1)
}

func TestLLMGenerator_Generate_StampsServerGeneratedIDs(t *testing.T) {
	// The LLM JSON has no "id" field — the generator must assign a non-empty UUID.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openAIResp(validOptionMultipleJSON))
	}
	gen := newGenerator(t, handler, 0, time.Millisecond)

	qs, err := gen.Generate(context.Background(), defaultParams)

	require.NoError(t, err)
	require.Len(t, qs, 1)
	assert.NotEmpty(t, qs[0].ID, "generator must stamp a server-generated UUID")
	// UUID v4 format: 8-4-4-4-12 hex chars separated by dashes
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, qs[0].ID)
}

func TestLLMGenerator_Generate_OverridesLLMFieldsWithParams(t *testing.T) {
	// wrongFieldsJSON has subject="language" and grade=5;
	// params say SubjectMathematics and grade=3.
	// Generator must stamp the params values after schema validation.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openAIResp(wrongFieldsJSON))
	}
	gen := newGenerator(t, handler, 0, time.Millisecond)

	qs, err := gen.Generate(context.Background(), defaultParams)

	require.NoError(t, err)
	require.Len(t, qs, 1)
	assert.Equal(t, domain.SubjectMathematics, qs[0].Subject, "subject must match params, not LLM output")
	assert.Equal(t, 3, qs[0].Grade, "grade must match params, not LLM output")
	assert.Equal(t, domain.GameTypeOptionMultiple, qs[0].Type, "type must match params, not LLM output")
}

func TestLLMGenerator_Generate_UniqueIDsPerQuestion(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openAIResp(twoQuestionsJSON))
	}
	gen := newGenerator(t, handler, 0, time.Millisecond)

	params := questions.GenerateParams{
		Subject: domain.SubjectMathematics,
		Grade:   3,
		Type:    domain.GameTypeOptionMultiple,
		Topic:   "multiplication",
		Count:   2,
	}

	qs, err := gen.Generate(context.Background(), params)

	require.NoError(t, err)
	require.Len(t, qs, 2)
	assert.NotEmpty(t, qs[0].ID)
	assert.NotEmpty(t, qs[1].ID)
	assert.NotEqual(t, qs[0].ID, qs[1].ID, "each question must receive a unique UUID")
}

func TestLLMGenerator_Generate_RetriesOnParseError(t *testing.T) {
	var callCount atomic.Int32
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if callCount.Add(1) == 1 {
			_, _ = w.Write(openAIResp("not valid json at all"))
		} else {
			_, _ = w.Write(openAIResp(validOptionMultipleJSON))
		}
	}
	gen := newGenerator(t, handler, 2, time.Millisecond)

	qs, err := gen.Generate(context.Background(), defaultParams)

	require.NoError(t, err)
	require.Len(t, qs, 1)
	assert.Equal(t, int32(2), callCount.Load(), "must make exactly 2 LLM calls (1 failed + 1 retry)")
}

func TestLLMGenerator_Generate_RetriesOnSchemaValidationError(t *testing.T) {
	var callCount atomic.Int32
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if callCount.Add(1) == 1 {
			// Parses as JSON but fails schema (missing required fields).
			_, _ = w.Write(openAIResp(invalidSchemaJSON))
		} else {
			_, _ = w.Write(openAIResp(validOptionMultipleJSON))
		}
	}
	gen := newGenerator(t, handler, 2, time.Millisecond)

	qs, err := gen.Generate(context.Background(), defaultParams)

	require.NoError(t, err)
	require.Len(t, qs, 1)
	assert.Equal(t, int32(2), callCount.Load(), "must make exactly 2 LLM calls (schema fail + retry)")
}

func TestLLMGenerator_Generate_ExhaustsRetries_ReturnsErrNoValidQuestions(t *testing.T) {
	var callCount atomic.Int32
	handler := func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openAIResp("totally invalid"))
	}
	// maxRetries=2: initial attempt + 2 retries = 3 total LLM calls.
	gen := newGenerator(t, handler, 2, time.Millisecond)

	_, err := gen.Generate(context.Background(), defaultParams)

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNoValidQuestions))
	assert.Equal(t, int32(3), callCount.Load(), "must attempt initial call + maxRetries retries")
}

func TestLLMGenerator_Generate_RejectsNonLLMType(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("LLM should not be called for a non-LLM game type")
	}
	gen := newGenerator(t, handler, 2, time.Millisecond)

	_, err := gen.Generate(context.Background(), questions.GenerateParams{
		Subject: domain.SubjectMathematics,
		Grade:   3,
		Type:    domain.GameTypeQuickCalc,
		Count:   1,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not handle type")
}

func TestLLMGenerator_Generate_RetryDelayIsApplied(t *testing.T) {
	// Use a measurable delay so we can assert elapsed time >= retryDelay.
	const retryDelay = 50 * time.Millisecond

	var callCount atomic.Int32
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if callCount.Add(1) == 1 {
			_, _ = w.Write(openAIResp("invalid json"))
		} else {
			_, _ = w.Write(openAIResp(validOptionMultipleJSON))
		}
	}
	gen := newGenerator(t, handler, 2, retryDelay)

	start := time.Now()
	_, err := gen.Generate(context.Background(), defaultParams)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, retryDelay,
		"elapsed time must be at least retryDelay (%s), got %s", retryDelay, elapsed)
}

func TestLLMGenerator_Generate_ContextCancelledDuringRetryDelay(t *testing.T) {
	// retryDelay is much longer than the context deadline, so the context fires
	// during the select inside the retry loop — Generate must return early.
	const retryDelay = 5 * time.Second

	var callCount atomic.Int32
	handler := func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openAIResp("invalid json")) // always invalid → triggers retry delay
	}
	gen := newGenerator(t, handler, 2, retryDelay)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := gen.Generate(ctx, defaultParams)

	require.Error(t, err)
	// Must NOT have exhausted all retries — the context fired before the delay expired.
	assert.False(t, errors.Is(err, domain.ErrNoValidQuestions),
		"should not exhaust retries: context must cancel during the retry delay")
	// Only the first LLM call should have been made; the retry was blocked by the delay.
	assert.Equal(t, int32(1), callCount.Load(), "only 1 LLM call expected before context cancelled")
}

func TestLLMGenerator_Generate_RateLimit_ContextDeadlineExceededDuringBackoff(t *testing.T) {
	// Server always returns 429. completeWithRateLimitBackoff waits 30 s before its first
	// RL retry; the 50 ms context deadline fires during that wait.
	// Generate must detect ctx.Err() == DeadlineExceeded and return ErrLLMTimeout.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}
	// maxRetries=0 so Generate only calls completeWithRateLimitBackoff once.
	gen := newGenerator(t, handler, 0, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := gen.Generate(ctx, defaultParams)

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrLLMTimeout),
		"rate-limit backoff cut by deadline must surface as ErrLLMTimeout, got: %v", err)
}
