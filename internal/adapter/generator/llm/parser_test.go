package llm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/llm"
)

const minimalQuestionJSON = `[{
	"type": "option_multiple",
	"subject": "mathematics",
	"grade": 3,
	"topic": "multiplication",
	"statement": "¿Cuánto es 4 × 6?",
	"options": [
		{"id":"A","text":"20"},{"id":"B","text":"24"},{"id":"C","text":"26"},{"id":"D","text":"16"}
	],
	"correctAnswers": ["B"],
	"meta": {"difficulty": "easy", "timeLimitMs": 15000, "tags": []}
}]`

func TestParseQuestions_BareArray(t *testing.T) {
	raw, qs, err := llm.ParseQuestions(minimalQuestionJSON)
	require.NoError(t, err)
	assert.Len(t, qs, 1)
	assert.NotEmpty(t, raw)
}

func TestParseQuestions_MarkdownFenceJSON(t *testing.T) {
	fenced := "```json\n" + minimalQuestionJSON + "\n```"
	_, qs, err := llm.ParseQuestions(fenced)
	require.NoError(t, err)
	assert.Len(t, qs, 1)
}

func TestParseQuestions_MarkdownFencePlain(t *testing.T) {
	fenced := "```\n" + minimalQuestionJSON + "\n```"
	_, qs, err := llm.ParseQuestions(fenced)
	require.NoError(t, err)
	assert.Len(t, qs, 1)
}

func TestParseQuestions_PreambleBeforeArray(t *testing.T) {
	withPreamble := "Sure, here are the questions:\n" + minimalQuestionJSON
	_, qs, err := llm.ParseQuestions(withPreamble)
	require.NoError(t, err)
	assert.Len(t, qs, 1)
}

func TestParseQuestions_PostambleAfterArray(t *testing.T) {
	withPostamble := minimalQuestionJSON + "\nHope that helps!"
	_, qs, err := llm.ParseQuestions(withPostamble)
	require.NoError(t, err)
	assert.Len(t, qs, 1)
}

func TestParseQuestions_InvalidJSON(t *testing.T) {
	raw, qs, err := llm.ParseQuestions(`[not valid json`)
	assert.Error(t, err)
	assert.Nil(t, qs)
	assert.NotEmpty(t, raw)
}

func TestParseQuestions_EmptyString(t *testing.T) {
	_, qs, err := llm.ParseQuestions("")
	assert.Error(t, err)
	assert.Nil(t, qs)
}

func TestParseQuestions_PreambleAndFence(t *testing.T) {
	text := "Sure!\n```json\n" + minimalQuestionJSON + "\n```"
	_, qs, err := llm.ParseQuestions(text)
	require.NoError(t, err)
	assert.Len(t, qs, 1)
}
