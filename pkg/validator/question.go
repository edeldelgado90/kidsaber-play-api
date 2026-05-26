package validator

import (
	"fmt"
	"strings"
	"sync"

	"github.com/xeipuuv/gojsonschema"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// Schemas are compiled once at startup.
var (
	once    sync.Once
	schemas map[domain.GameType]*gojsonschema.Schema
)

// JSON Schema definitions for each LLM game type.
// quick_calculation is procedural and does not need schema validation.
const optionMultipleSchemaStr = `{
  "type": "array",
  "minItems": 1,
  "items": {
    "type": "object",
    "required": ["id", "type", "subject", "grade", "topic", "statement", "options", "correctAnswers", "meta"],
    "properties": {
      "id":        { "type": "string", "minLength": 1 },
      "type":      { "type": "string", "enum": ["option_multiple"] },
      "subject":   { "type": "string", "enum": ["mathematics","language","english","science"] },
      "grade":     { "type": "integer", "minimum": 1, "maximum": 6 },
      "topic":     { "type": "string", "minLength": 1 },
      "statement": { "type": "string", "minLength": 1 },
      "options": {
        "type": "array", "minItems": 4, "maxItems": 4,
        "items": {
          "type": "object",
          "required": ["id","text"],
          "properties": {
            "id":   { "type": "string", "minLength": 1 },
            "text": { "type": "string", "minLength": 1 }
          }
        }
      },
      "correctAnswers": {
        "type": "array", "minItems": 1, "maxItems": 1,
        "items": { "type": "string", "minLength": 1 }
      },
      "meta": {
        "type": "object",
        "required": ["difficulty","timeLimitMs","tags"],
        "properties": {
          "difficulty":  { "type": "string", "enum": ["easy","medium","hard"] },
          "timeLimitMs": { "type": "integer", "minimum": 0 },
          "tags":        { "type": "array", "items": { "type": "string" } }
        }
      }
    }
  }
}`

const fillInTheBlanksSchemaStr = `{
  "type": "array",
  "minItems": 1,
  "items": {
    "type": "object",
    "required": ["id","type","subject","grade","topic","statement","options","correctAnswers","meta"],
    "properties": {
      "id":        { "type": "string", "minLength": 1 },
      "type":      { "type": "string", "enum": ["fill_in_the_blanks"] },
      "subject":   { "type": "string", "enum": ["mathematics","language","english","science"] },
      "grade":     { "type": "integer", "minimum": 1, "maximum": 6 },
      "topic":     { "type": "string", "minLength": 1 },
      "statement": { "type": "string", "minLength": 3 },
      "options": {
        "type": "array", "minItems": 3, "maxItems": 3,
        "items": {
          "type": "object",
          "required": ["id","text"],
          "properties": {
            "id":   { "type": "string", "minLength": 1 },
            "text": { "type": "string", "minLength": 1 }
          }
        }
      },
      "correctAnswers": {
        "type": "array", "minItems": 1, "maxItems": 1,
        "items": { "type": "string", "minLength": 1 }
      },
      "meta": {
        "type": "object",
        "required": ["difficulty","timeLimitMs","tags"],
        "properties": {
          "difficulty":  { "type": "string", "enum": ["easy","medium","hard"] },
          "timeLimitMs": { "type": "integer", "minimum": 0 },
          "tags":        { "type": "array", "items": { "type": "string" } }
        }
      }
    }
  }
}`

const matchingSchemaStr = `{
  "type": "array",
  "minItems": 1,
  "items": {
    "type": "object",
    "required": ["id","type","subject","grade","topic","statement","pairs","correctAnswers","meta"],
    "properties": {
      "id":        { "type": "string", "minLength": 1 },
      "type":      { "type": "string", "enum": ["matching"] },
      "subject":   { "type": "string", "enum": ["mathematics","language","english","science"] },
      "grade":     { "type": "integer", "minimum": 1, "maximum": 6 },
      "topic":     { "type": "string", "minLength": 1 },
      "statement": { "type": "string", "minLength": 1 },
      "pairs": {
        "type": "object",
        "required": ["left","right"],
        "properties": {
          "left": {
            "type": "array", "minItems": 3, "maxItems": 4,
            "items": {
              "type": "object", "required": ["id","text"],
              "properties": {
                "id":   { "type": "string", "minLength": 1 },
                "text": { "type": "string", "minLength": 1 }
              }
            }
          },
          "right": {
            "type": "array", "minItems": 3, "maxItems": 4,
            "items": {
              "type": "object", "required": ["id","text"],
              "properties": {
                "id":   { "type": "string", "minLength": 1 },
                "text": { "type": "string", "minLength": 1 }
              }
            }
          }
        }
      },
      "correctAnswers": {
        "type": "array", "minItems": 3, "maxItems": 4,
        "items": {
          "type": "object",
          "required": ["leftId","rightId"],
          "properties": {
            "leftId":  { "type": "string", "minLength": 1 },
            "rightId": { "type": "string", "minLength": 1 }
          }
        }
      },
      "meta": {
        "type": "object",
        "required": ["difficulty","timeLimitMs","tags"],
        "properties": {
          "difficulty":  { "type": "string", "enum": ["easy","medium","hard"] },
          "timeLimitMs": { "type": "integer", "minimum": 0 },
          "tags":        { "type": "array", "items": { "type": "string" } }
        }
      }
    }
  }
}`

// QuestionValidator validates raw LLM JSON output against per-type schemas.
type QuestionValidator struct{}

// NewQuestionValidator compiles schemas once and returns a validator instance.
func NewQuestionValidator() (*QuestionValidator, error) {
	var compileErr error
	once.Do(func() {
		schemas = make(map[domain.GameType]*gojsonschema.Schema)
		defs := map[domain.GameType]string{
			domain.GameTypeOptionMultiple:  optionMultipleSchemaStr,
			domain.GameTypeFillInTheBlanks: fillInTheBlanksSchemaStr,
			domain.GameTypeMatching:        matchingSchemaStr,
		}
		for gt, def := range defs {
			s, err := gojsonschema.NewSchema(gojsonschema.NewStringLoader(def))
			if err != nil {
				compileErr = fmt.Errorf("compiling schema for %s: %w", gt, err)
				return
			}
			schemas[gt] = s
		}
	})
	if compileErr != nil {
		return nil, compileErr
	}
	return &QuestionValidator{}, nil
}

// ValidateRaw validates the raw JSON byte slice (an array) against the schema for gameType.
// Returns a non-nil error with human-readable details if validation fails.
func (v *QuestionValidator) ValidateRaw(gameType domain.GameType, raw []byte) error {
	schema, ok := schemas[gameType]
	if !ok {
		// No schema for this type (e.g. quick_calculation) — skip validation.
		return nil
	}

	result, err := schema.Validate(gojsonschema.NewBytesLoader(raw))
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}
	if !result.Valid() {
		errs := make([]string, 0, len(result.Errors()))
		for _, re := range result.Errors() {
			errs = append(errs, re.String())
		}
		return fmt.Errorf("schema validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ValidationErrors extracts a human-readable list of errors for the retry prompt.
func (v *QuestionValidator) ValidationErrors(gameType domain.GameType, raw []byte) string {
	if err := v.ValidateRaw(gameType, raw); err != nil {
		return err.Error()
	}
	return ""
}
