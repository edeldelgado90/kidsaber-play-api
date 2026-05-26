package procedural

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/google/uuid"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

// gradeRange defines the operand ranges and allowed operations for each grade band.
type gradeRange struct {
	minVal   int
	maxVal   int
	ops      []string // "+", "-", "*", "/"
}

// rangeConfig returns the appropriate number range and operations for a grade.
// Grade 1–2: addition/subtraction within 20; numbers 1–10
// Grade 3–4: addition/subtraction within 1000; multiplication tables 2–10; up to 100
// Grade 5–6: all operations; numbers up to 10,000
func rangeConfig(grade int) gradeRange {
	switch {
	case grade <= 2:
		return gradeRange{minVal: 1, maxVal: 10, ops: []string{"+", "-"}}
	case grade <= 4:
		return gradeRange{minVal: 1, maxVal: 100, ops: []string{"+", "-", "*"}}
	default:
		return gradeRange{minVal: 1, maxVal: 10000, ops: []string{"+", "-", "*", "/"}}
	}
}

// topicForOp returns the curriculum topic_id for a given arithmetic operator.
func topicForOp(op string, grade int) string {
	switch op {
	case "+":
		if grade <= 2 {
			return "addition_within_20"
		}
		return "addition_subtraction_large_numbers"
	case "-":
		if grade <= 2 {
			return "subtraction_within_20"
		}
		return "addition_subtraction_large_numbers"
	case "*":
		if grade <= 2 {
			return "multiplication_tables_2_3_4_5"
		}
		return "multiplication_tables_complete"
	case "/":
		return "division_exact_inexact"
	default:
		return "arithmetic"
	}
}

// metaForGrade returns reasonable difficulty and time metadata for a grade.
func metaForGrade(grade int) domain.QuestionMeta {
	var diff domain.Difficulty
	var timeMs int

	switch {
	case grade <= 2:
		diff = domain.DifficultyEasy
		timeMs = 10000
	case grade <= 4:
		diff = domain.DifficultyMedium
		timeMs = 8000
	default:
		diff = domain.DifficultyHard
		timeMs = 6000
	}

	return domain.QuestionMeta{
		Difficulty:  diff,
		TimeLimitMs: timeMs,
		Tags:        []string{"arithmetic", "mental_calculation"},
	}
}

// generateDistractors creates 3 plausible wrong answers near the correct value.
// The distractors are deduplicated and do not equal the correct answer.
func generateDistractors(correct, count int) []int {
	seen := map[int]bool{correct: true}
	var distractors []int

	offsets := []int{1, 2, 3, 4, 5, 10, 11, 12}
	for _, off := range offsets {
		if len(distractors) >= count {
			break
		}
		for _, candidate := range []int{correct + off, correct - off} {
			if candidate > 0 && !seen[candidate] {
				seen[candidate] = true
				distractors = append(distractors, candidate)
				if len(distractors) >= count {
					break
				}
			}
		}
	}

	// Fallback: add random nearby values if we still need more
	for len(distractors) < count {
		candidate := correct + rand.IntN(20) - 10
		if candidate > 0 && !seen[candidate] {
			seen[candidate] = true
			distractors = append(distractors, candidate)
		}
	}

	return distractors[:count]
}

// generateOperation produces a valid arithmetic expression and result for a grade.
func generateOperation(cfg gradeRange) (a, b int, op string, result int) {
	op = cfg.ops[rand.IntN(len(cfg.ops))]

	switch op {
	case "+":
		a = rand.IntN(cfg.maxVal) + 1
		b = rand.IntN(cfg.maxVal) + 1
		result = a + b

	case "-":
		a = rand.IntN(cfg.maxVal) + 1
		b = rand.IntN(a) + 1 // ensure non-negative result
		result = a - b

	case "*":
		// Keep multiplication in table range (2–10 × 2–10)
		a = rand.IntN(9) + 2
		b = rand.IntN(9) + 2
		result = a * b

	case "/":
		// Generate divisible pairs: result * divisor = dividend
		result = rand.IntN(20) + 1
		b = rand.IntN(9) + 2
		a = result * b // a / b = result (exact division)
	}

	return
}

// CalculatorGenerator implements questions.QuestionGenerator for quick_calculation.
// It is always procedural — no LLM call, no DB read.
type CalculatorGenerator struct{}

// NewCalculatorGenerator creates a CalculatorGenerator.
func NewCalculatorGenerator() *CalculatorGenerator {
	return &CalculatorGenerator{}
}

// Generate produces `params.Count` procedurally generated arithmetic questions.
func (g *CalculatorGenerator) Generate(_ context.Context, params questions.GenerateParams) ([]domain.Question, error) {
	cfg := rangeConfig(params.Grade)
	meta := metaForGrade(params.Grade)

	qs := make([]domain.Question, 0, params.Count)

	for i := 0; i < params.Count; i++ {
		a, b, op, result := generateOperation(cfg)
		expression := fmt.Sprintf("%d %s %d", a, op, b)
		topic := topicForOp(op, params.Grade)

		// correctAnswers: [result]
		correctAnswers, err := json.Marshal([]int{result})
		if err != nil {
			return nil, fmt.Errorf("marshalling correctAnswers: %w", err)
		}

		// Build option list: 1 correct + 3 distractors
		distractors := generateDistractors(result, 3)
		optionValues := append([]int{result}, distractors...)

		// Shuffle option values
		rand.Shuffle(len(optionValues), func(x, y int) {
			optionValues[x], optionValues[y] = optionValues[y], optionValues[x]
		})

		ids := []string{"A", "B", "C", "D"}
		opts := make([]domain.Option, len(optionValues))
		correctID := ""
		for j, val := range optionValues {
			opts[j] = domain.Option{ID: ids[j], Text: fmt.Sprintf("%d", val)}
			if val == result {
				correctID = ids[j]
			}
		}

		// Update correctAnswers to use the option ID
		correctAnswers, err = json.Marshal([]string{correctID})
		if err != nil {
			return nil, fmt.Errorf("marshalling correctAnswers IDs: %w", err)
		}

		q := domain.Question{
			ID:             uuid.New().String(),
			Type:           domain.GameTypeQuickCalc,
			Subject:        domain.SubjectMathematics,
			Grade:          params.Grade,
			Topic:          topic,
			Statement:      "Resuelve la operación.",
			Expression:     expression,
			Options:        opts,
			CorrectAnswers: json.RawMessage(correctAnswers),
			Meta:           meta,
		}

		qs = append(qs, q)
	}

	return qs, nil
}
