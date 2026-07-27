package llm_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/llm"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

func TestBuildPrompt_OptionMultiple(t *testing.T) {
	data := llm.PromptData{
		SubjectSpanish: "Matemáticas",
		Subject:        "mathematics",
		Grade:          3,
		AgeMin:         8,
		AgeMax:         9,
		Topic:          "multiplication_tables",
		Count:          5,
	}
	prompt, err := llm.BuildPrompt(domain.GameTypeOptionMultiple, data)
	require.NoError(t, err)
	assert.Contains(t, prompt, "Matemáticas")
	assert.Contains(t, prompt, "mathematics")
	assert.Contains(t, prompt, "3")
	assert.Contains(t, prompt, "multiplication_tables")
	assert.Contains(t, prompt, "5")
	assert.Contains(t, prompt, "option_multiple")
}

func TestBuildPrompt_FillInTheBlanks(t *testing.T) {
	data := llm.PromptData{
		SubjectSpanish: "Lengua Castellana y Literatura",
		Subject:        "language",
		Grade:          4,
		AgeMin:         9,
		AgeMax:         10,
		Topic:          "verb_tense",
		Count:          3,
	}
	prompt, err := llm.BuildPrompt(domain.GameTypeFillInTheBlanks, data)
	require.NoError(t, err)
	assert.Contains(t, prompt, "fill_in_the_blanks")
	assert.Contains(t, prompt, "verb_tense")
	assert.Contains(t, prompt, "3")
}

func TestBuildPrompt_Matching(t *testing.T) {
	data := llm.PromptData{
		SubjectSpanish: "Inglés",
		Subject:        "english",
		Grade:          2,
		AgeMin:         7,
		AgeMax:         8,
		Topic:          "animals",
		Count:          2,
	}
	prompt, err := llm.BuildPrompt(domain.GameTypeMatching, data)
	require.NoError(t, err)
	assert.Contains(t, prompt, "matching")
	assert.Contains(t, prompt, "animals")
}

func TestBuildPrompt_UnknownGameType(t *testing.T) {
	data := llm.PromptData{Subject: "mathematics", Grade: 1, Count: 1}
	_, err := llm.BuildPrompt("quick_calculation", data)
	assert.ErrorContains(t, err, "no prompt template for game type")
}

func TestBuildRetryPrompt(t *testing.T) {
	originalPrompt := "Original prompt here"
	validationErrors := "field X is missing"
	count := 4

	result, err := llm.BuildRetryPrompt(originalPrompt, validationErrors, count)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(result, "CORRECCIÓN"), "expected correction header at start")
	assert.Contains(t, result, validationErrors)
	assert.Contains(t, result, originalPrompt)

	headerIdx := strings.Index(result, "CORRECCIÓN")
	promptIdx := strings.Index(result, originalPrompt)
	assert.Less(t, headerIdx, promptIdx, "correction header must appear before original prompt")
}
