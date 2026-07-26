package validator_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/pkg/validator"
)

// ── fixtures ──────────────────────────────────────────────────────────────────

const validOptionMultiple = `[{
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

const validFillInTheBlanks = `[{
	"type": "fill_in_the_blanks",
	"subject": "language",
	"grade": 3,
	"topic": "verb_tense",
	"statement": "Mi hermano ____ al colegio todos los días.",
	"options": [
		{"id": "A", "text": "va"},
		{"id": "B", "text": "voy"},
		{"id": "C", "text": "fui"}
	],
	"correctAnswers": ["A"],
	"meta": {"difficulty": "easy", "timeLimitMs": 20000, "tags": ["verb"]}
}]`

const validMatching = `[{
	"type": "matching",
	"subject": "science",
	"grade": 3,
	"topic": "ecosystems",
	"statement": "Une cada animal con su tipo.",
	"pairs": {
		"left":  [{"id": "L1", "text": "Perro"}, {"id": "L2", "text": "Águila"}, {"id": "L3", "text": "Salmón"}],
		"right": [{"id": "R1", "text": "Ave"},   {"id": "R2", "text": "Pez"},    {"id": "R3", "text": "Mamífero"}]
	},
	"correctAnswers": [
		{"leftId": "L1", "rightId": "R3"},
		{"leftId": "L2", "rightId": "R1"},
		{"leftId": "L3", "rightId": "R2"}
	],
	"meta": {"difficulty": "medium", "timeLimitMs": 30000, "tags": ["animals"]}
}]`

// ── helpers ───────────────────────────────────────────────────────────────────

func mustValidator(t *testing.T) *validator.QuestionValidator {
	t.Helper()
	v, err := validator.NewQuestionValidator()
	require.NoError(t, err)
	require.NotNil(t, v)
	return v
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestNewQuestionValidator(t *testing.T) {
	v, err := validator.NewQuestionValidator()
	assert.NoError(t, err)
	assert.NotNil(t, v)
}

func TestValidateRaw_OptionMultiple(t *testing.T) {
	v := mustValidator(t)

	t.Run("valid JSON", func(t *testing.T) {
		assert.NoError(t, v.ValidateRaw(domain.GameTypeOptionMultiple, []byte(validOptionMultiple)))
	})

	t.Run("missing statement", func(t *testing.T) {
		var obj []map[string]any
		require.NoError(t, json.Unmarshal([]byte(validOptionMultiple), &obj))
		delete(obj[0], "statement")
		raw, _ := json.Marshal(obj)
		err := v.ValidateRaw(domain.GameTypeOptionMultiple, raw)
		assert.ErrorContains(t, err, "schema validation failed")
	})

	t.Run("invalid difficulty enum", func(t *testing.T) {
		var obj []map[string]any
		require.NoError(t, json.Unmarshal([]byte(validOptionMultiple), &obj))
		obj[0]["meta"].(map[string]any)["difficulty"] = "super-hard"
		raw, _ := json.Marshal(obj)
		err := v.ValidateRaw(domain.GameTypeOptionMultiple, raw)
		assert.ErrorContains(t, err, "schema validation failed")
	})

	t.Run("wrong number of options (3 instead of 4)", func(t *testing.T) {
		var obj []map[string]any
		require.NoError(t, json.Unmarshal([]byte(validOptionMultiple), &obj))
		opts := obj[0]["options"].([]any)
		obj[0]["options"] = opts[:3]
		raw, _ := json.Marshal(obj)
		err := v.ValidateRaw(domain.GameTypeOptionMultiple, raw)
		assert.ErrorContains(t, err, "schema validation failed")
	})
}

func TestValidateRaw_FillInTheBlanks(t *testing.T) {
	v := mustValidator(t)

	t.Run("valid JSON", func(t *testing.T) {
		assert.NoError(t, v.ValidateRaw(domain.GameTypeFillInTheBlanks, []byte(validFillInTheBlanks)))
	})

	t.Run("4 options exceeds max of 3", func(t *testing.T) {
		var obj []map[string]any
		require.NoError(t, json.Unmarshal([]byte(validFillInTheBlanks), &obj))
		extraOpt := map[string]any{"id": "D", "text": "extra"}
		opts := append(obj[0]["options"].([]any), extraOpt)
		obj[0]["options"] = opts
		raw, _ := json.Marshal(obj)
		err := v.ValidateRaw(domain.GameTypeFillInTheBlanks, raw)
		assert.ErrorContains(t, err, "schema validation failed")
	})

	t.Run("missing required field", func(t *testing.T) {
		var obj []map[string]any
		require.NoError(t, json.Unmarshal([]byte(validFillInTheBlanks), &obj))
		delete(obj[0], "topic")
		raw, _ := json.Marshal(obj)
		err := v.ValidateRaw(domain.GameTypeFillInTheBlanks, raw)
		assert.ErrorContains(t, err, "schema validation failed")
	})
}

func TestValidateRaw_Matching(t *testing.T) {
	v := mustValidator(t)

	t.Run("valid JSON", func(t *testing.T) {
		assert.NoError(t, v.ValidateRaw(domain.GameTypeMatching, []byte(validMatching)))
	})

	t.Run("correctAnswers has 2 entries (min is 3)", func(t *testing.T) {
		var obj []map[string]any
		require.NoError(t, json.Unmarshal([]byte(validMatching), &obj))
		answers := obj[0]["correctAnswers"].([]any)
		obj[0]["correctAnswers"] = answers[:2]
		raw, _ := json.Marshal(obj)
		err := v.ValidateRaw(domain.GameTypeMatching, raw)
		assert.ErrorContains(t, err, "schema validation failed")
	})

	t.Run("missing pairs", func(t *testing.T) {
		var obj []map[string]any
		require.NoError(t, json.Unmarshal([]byte(validMatching), &obj))
		delete(obj[0], "pairs")
		raw, _ := json.Marshal(obj)
		err := v.ValidateRaw(domain.GameTypeMatching, raw)
		assert.ErrorContains(t, err, "schema validation failed")
	})
}

func TestValidateRaw_NoSchema(t *testing.T) {
	v := mustValidator(t)

	t.Run("quick_calculation skips validation", func(t *testing.T) {
		assert.NoError(t, v.ValidateRaw(domain.GameTypeQuickCalc, []byte(`[{}]`)))
	})

	t.Run("unknown game type skips validation", func(t *testing.T) {
		assert.NoError(t, v.ValidateRaw("unknown_type", []byte(`[{}]`)))
	})
}

func TestValidationErrors(t *testing.T) {
	v := mustValidator(t)

	t.Run("valid input returns empty string", func(t *testing.T) {
		result := v.ValidationErrors(domain.GameTypeOptionMultiple, []byte(validOptionMultiple))
		assert.Equal(t, "", result)
	})

	t.Run("invalid input returns error message", func(t *testing.T) {
		var obj []map[string]any
		require.NoError(t, json.Unmarshal([]byte(validOptionMultiple), &obj))
		delete(obj[0], "statement")
		raw, _ := json.Marshal(obj)
		result := v.ValidationErrors(domain.GameTypeOptionMultiple, raw)
		assert.True(t, strings.Contains(result, "schema validation failed"), "expected error in result, got: %s", result)
	})
}
