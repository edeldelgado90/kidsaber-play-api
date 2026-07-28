package validator_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/pkg/validator"
)

// These cases are all schema-valid. They cover the defects the JSON Schema cannot
// express: questions that render badly or that a child cannot answer correctly.

func TestValidateRaw_SemanticRules(t *testing.T) {
	tests := []struct {
		name     string
		gameType domain.GameType
		raw      string
		wantErr  string
	}{
		{
			name:     "fill_in_the_blanks with three underscores",
			gameType: domain.GameTypeFillInTheBlanks,
			raw: `[{
				"type": "fill_in_the_blanks", "subject": "language", "grade": 3, "topic": "verb",
				"statement": "Mi hermano ___ al colegio.",
				"options": [{"id":"A","text":"va"},{"id":"B","text":"voy"},{"id":"C","text":"fui"}],
				"correctAnswers": ["A"],
				"meta": {"difficulty":"easy","timeLimitMs":20000,"tags":["verb"]}
			}]`,
			wantErr: "blank marker must be exactly",
		},
		{
			name:     "fill_in_the_blanks with no blank at all",
			gameType: domain.GameTypeFillInTheBlanks,
			raw: `[{
				"type": "fill_in_the_blanks", "subject": "mathematics", "grade": 4, "topic": "fractions",
				"statement": "¿Cuál es igual a 9/12?",
				"options": [{"id":"A","text":"1/2"},{"id":"B","text":"2/3"},{"id":"C","text":"3/4"}],
				"correctAnswers": ["C"],
				"meta": {"difficulty":"medium","timeLimitMs":18000,"tags":["fractions"]}
			}]`,
			wantErr: "must contain the blank marker",
		},
		{
			name:     "fill_in_the_blanks with two blanks",
			gameType: domain.GameTypeFillInTheBlanks,
			raw: `[{
				"type": "fill_in_the_blanks", "subject": "language", "grade": 3, "topic": "verb",
				"statement": "El ____ come ____ todos los días.",
				"options": [{"id":"A","text":"niño"},{"id":"B","text":"perro"},{"id":"C","text":"gato"}],
				"correctAnswers": ["A"],
				"meta": {"difficulty":"easy","timeLimitMs":20000,"tags":["verb"]}
			}]`,
			wantErr: "exactly one",
		},
		{
			name:     "option_multiple with two identical options",
			gameType: domain.GameTypeOptionMultiple,
			raw: `[{
				"type": "option_multiple", "subject": "mathematics", "grade": 3, "topic": "division",
				"statement": "¿Cuánto es 16 ÷ 4?",
				"options": [{"id":"A","text":"4"},{"id":"B","text":"4"},{"id":"C","text":"3"},{"id":"D","text":"5"}],
				"correctAnswers": ["A"],
				"meta": {"difficulty":"easy","timeLimitMs":15000,"tags":["division"]}
			}]`,
			wantErr: "duplicate option text",
		},
		{
			name:     "option_multiple whose correct answer is not an option",
			gameType: domain.GameTypeOptionMultiple,
			raw: `[{
				"type": "option_multiple", "subject": "mathematics", "grade": 3, "topic": "division",
				"statement": "¿Cuánto es 16 ÷ 4?",
				"options": [{"id":"A","text":"4"},{"id":"B","text":"6"},{"id":"C","text":"3"},{"id":"D","text":"5"}],
				"correctAnswers": ["E"],
				"meta": {"difficulty":"easy","timeLimitMs":15000,"tags":["division"]}
			}]`,
			wantErr: "unknown option id",
		},
		{
			name:     "statement padded with whitespace",
			gameType: domain.GameTypeOptionMultiple,
			raw: `[{
				"type": "option_multiple", "subject": "language", "grade": 4, "topic": "spelling",
				"statement": "Palabra con «je»: ",
				"options": [{"id":"A","text":"muger"},{"id":"B","text":"mujer"},{"id":"C","text":"mugier"},{"id":"D","text":"mujier"}],
				"correctAnswers": ["B"],
				"meta": {"difficulty":"medium","timeLimitMs":18000,"tags":["spelling"]}
			}]`,
			wantErr: "trailing whitespace",
		},
		{
			name:     "matching where two left items share one right item",
			gameType: domain.GameTypeMatching,
			raw: `[{
				"type": "matching", "subject": "mathematics", "grade": 5, "topic": "fractions",
				"statement": "Une división y resultado.",
				"pairs": {
					"left":  [{"id":"L1","text":"1/2 ÷ 1/4"},{"id":"L2","text":"3/5 ÷ 3/10"},{"id":"L3","text":"5/6 ÷ 5/12"}],
					"right": [{"id":"R1","text":"2"},{"id":"R2","text":"2"},{"id":"R3","text":"2"}]
				},
				"correctAnswers": [
					{"leftId":"L1","rightId":"R1"},
					{"leftId":"L2","rightId":"R1"},
					{"leftId":"L3","rightId":"R1"}
				],
				"meta": {"difficulty":"medium","timeLimitMs":25000,"tags":["fractions"]}
			}]`,
			wantErr: "used by more than one pair",
		},
		{
			name:     "matching referencing an unknown right id",
			gameType: domain.GameTypeMatching,
			raw: `[{
				"type": "matching", "subject": "science", "grade": 3, "topic": "animals",
				"statement": "Une cada animal con su tipo.",
				"pairs": {
					"left":  [{"id":"L1","text":"Perro"},{"id":"L2","text":"Águila"},{"id":"L3","text":"Salmón"}],
					"right": [{"id":"R1","text":"Ave"},{"id":"R2","text":"Pez"},{"id":"R3","text":"Mamífero"}]
				},
				"correctAnswers": [
					{"leftId":"L1","rightId":"R3"},
					{"leftId":"L2","rightId":"R1"},
					{"leftId":"L3","rightId":"R9"}
				],
				"meta": {"difficulty":"easy","timeLimitMs":25000,"tags":["animals"]}
			}]`,
			wantErr: "unknown rightId",
		},
	}

	v, err := validator.NewQuestionValidator()
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateRaw(tt.gameType, []byte(tt.raw))
			require.Error(t, err, "expected the defect to be rejected")
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// Classification questions legitimately repeat a label in column B, as long as
// each left item gets its own right item. Those must keep validating.
func TestValidateRaw_AllowsRepeatedRightLabels(t *testing.T) {
	const raw = `[{
		"type": "matching", "subject": "language", "grade": 1, "topic": "nouns_gender",
		"statement": "Une cada palabra con su género gramatical.",
		"pairs": {
			"left":  [{"id":"L1","text":"mesa"},{"id":"L2","text":"libro"},{"id":"L3","text":"silla"}],
			"right": [{"id":"R1","text":"femenino"},{"id":"R2","text":"masculino"},{"id":"R3","text":"femenino"}]
		},
		"correctAnswers": [
			{"leftId":"L1","rightId":"R1"},
			{"leftId":"L2","rightId":"R2"},
			{"leftId":"L3","rightId":"R3"}
		],
		"meta": {"difficulty":"easy","timeLimitMs":25000,"tags":["nouns"]}
	}]`

	v, err := validator.NewQuestionValidator()
	require.NoError(t, err)
	assert.NoError(t, v.ValidateRaw(domain.GameTypeMatching, []byte(raw)))
}

// Spelling questions ask the child to tell one capitalisation from another, so
// options that differ only in case must keep validating.
func TestValidateRaw_AllowsOptionsDifferingOnlyInCase(t *testing.T) {
	const raw = `[{
		"type": "option_multiple", "subject": "language", "grade": 1, "topic": "capitalisation",
		"statement": "¿Cuál nombre está escrito correctamente?",
		"options": [
			{"id":"A","text":"maría"},
			{"id":"B","text":"María"},
			{"id":"C","text":"MARÍa"},
			{"id":"D","text":"marÍa"}
		],
		"correctAnswers": ["B"],
		"meta": {"difficulty":"easy","timeLimitMs":15000,"tags":["capitalisation"]}
	}]`

	v, err := validator.NewQuestionValidator()
	require.NoError(t, err)
	assert.NoError(t, v.ValidateRaw(domain.GameTypeOptionMultiple, []byte(raw)))
}
