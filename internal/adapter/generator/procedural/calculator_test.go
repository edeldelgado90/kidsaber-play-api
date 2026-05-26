package procedural_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/procedural"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

func TestCalculatorGenerator_ReturnsCorrectCount(t *testing.T) {
	gen := procedural.NewCalculatorGenerator()

	for _, grade := range []int{1, 2, 3, 4, 5, 6} {
		qs, err := gen.Generate(context.Background(), questions.GenerateParams{
			Subject: domain.SubjectMathematics,
			Grade:   grade,
			Type:    domain.GameTypeQuickCalc,
			Count:   10,
		})
		require.NoError(t, err)
		assert.Len(t, qs, 10, "grade %d should return 10 questions", grade)
	}
}

func TestCalculatorGenerator_QuestionStructure(t *testing.T) {
	gen := procedural.NewCalculatorGenerator()
	qs, err := gen.Generate(context.Background(), questions.GenerateParams{
		Subject: domain.SubjectMathematics,
		Grade:   3,
		Type:    domain.GameTypeQuickCalc,
		Count:   5,
	})
	require.NoError(t, err)

	for _, q := range qs {
		assert.NotEmpty(t, q.ID)
		assert.Equal(t, domain.GameTypeQuickCalc, q.Type)
		assert.Equal(t, domain.SubjectMathematics, q.Subject)
		assert.Equal(t, 3, q.Grade)
		assert.NotEmpty(t, q.Expression, "expression must not be empty")
		assert.NotEmpty(t, q.Statement, "statement must not be empty")
		assert.Len(t, q.Options, 4, "must have 4 options")
		assert.NotEmpty(t, q.CorrectAnswers, "correctAnswers must not be empty")
		assert.NotEmpty(t, q.Topic, "topic must not be empty")
		assert.NotEmpty(t, q.Meta.Difficulty)
		assert.Greater(t, q.Meta.TimeLimitMs, 0)
	}
}

func TestCalculatorGenerator_CorrectAnswerIsAmongOptions(t *testing.T) {
	gen := procedural.NewCalculatorGenerator()
	qs, err := gen.Generate(context.Background(), questions.GenerateParams{
		Subject: domain.SubjectMathematics,
		Grade:   2,
		Type:    domain.GameTypeQuickCalc,
		Count:   20,
	})
	require.NoError(t, err)

	for _, q := range qs {
		var correctIDs []string
		require.NoError(t, json.Unmarshal(q.CorrectAnswers, &correctIDs))
		require.Len(t, correctIDs, 1, "quick_calculation should have exactly 1 correct answer")

		// The correct answer ID must be in the options
		found := false
		for _, opt := range q.Options {
			if opt.ID == correctIDs[0] {
				found = true
				break
			}
		}
		assert.True(t, found, "correct answer ID %s not found in options %+v", correctIDs[0], q.Options)
	}
}

func TestCalculatorGenerator_Grade1Range(t *testing.T) {
	gen := procedural.NewCalculatorGenerator()
	qs, err := gen.Generate(context.Background(), questions.GenerateParams{
		Subject: domain.SubjectMathematics,
		Grade:   1,
		Type:    domain.GameTypeQuickCalc,
		Count:   30,
	})
	require.NoError(t, err)

	for _, q := range qs {
		// Grade 1: only + and - operations
		assert.NotContains(t, q.Expression, "*", "grade 1 should not have multiplication: %s", q.Expression)
		assert.NotContains(t, q.Expression, "/", "grade 1 should not have division: %s", q.Expression)
	}
}

func TestCalculatorGenerator_UniqueIDs(t *testing.T) {
	gen := procedural.NewCalculatorGenerator()
	qs, err := gen.Generate(context.Background(), questions.GenerateParams{
		Subject: domain.SubjectMathematics,
		Grade:   4,
		Type:    domain.GameTypeQuickCalc,
		Count:   10,
	})
	require.NoError(t, err)

	seen := make(map[string]bool)
	for _, q := range qs {
		assert.False(t, seen[q.ID], "duplicate question ID: %s", q.ID)
		seen[q.ID] = true
	}
}
