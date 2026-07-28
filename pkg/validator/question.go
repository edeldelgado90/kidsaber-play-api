package validator

import (
	"encoding/json"
	"fmt"
	"regexp"
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
    "required": ["type", "subject", "grade", "topic", "statement", "options", "correctAnswers", "meta"],
    "properties": {
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
    "required": ["type","subject","grade","topic","statement","options","correctAnswers","meta"],
    "properties": {
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
    "required": ["type","subject","grade","topic","statement","pairs","correctAnswers","meta"],
    "properties": {
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
	return validateSemantics(gameType, raw)
}

// blankMarker is the exact placeholder the app splits the statement on. Any other
// run of underscores leaves stray characters on screen.
const blankMarker = "____"

var underscoreRun = regexp.MustCompile(`_+`)

// semanticItem mirrors the fields the semantic rules below need.
type semanticItem struct {
	Statement string `json:"statement"`
	Options   []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"options"`
	Pairs *struct {
		Left  []semanticPairItem `json:"left"`
		Right []semanticPairItem `json:"right"`
	} `json:"pairs"`
	CorrectAnswers json.RawMessage `json:"correctAnswers"`
}

type semanticPairItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// validateSemantics enforces the rules the JSON Schema cannot express: rules about
// how a question renders and whether a child can actually answer it correctly.
func validateSemantics(gameType domain.GameType, raw []byte) error {
	var items []semanticItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("semantic validation: %w", err)
	}

	var errs []string
	for i, it := range items {
		for _, msg := range semanticErrors(gameType, it) {
			errs = append(errs, fmt.Sprintf("item %d: %s", i, msg))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("semantic validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func semanticErrors(gameType domain.GameType, it semanticItem) []string {
	var errs []string

	if it.Statement != strings.TrimSpace(it.Statement) {
		errs = append(errs, "statement has leading or trailing whitespace")
	}

	if gameType == domain.GameTypeFillInTheBlanks {
		runs := underscoreRun.FindAllString(it.Statement, -1)
		switch {
		case len(runs) == 0:
			errs = append(errs, fmt.Sprintf("statement must contain the blank marker %q", blankMarker))
		case len(runs) > 1:
			errs = append(errs, fmt.Sprintf("statement must contain exactly one %q, found %d", blankMarker, len(runs)))
		case runs[0] != blankMarker:
			errs = append(errs, fmt.Sprintf("blank marker must be exactly %q, found %q", blankMarker, runs[0]))
		}
	}

	if gameType == domain.GameTypeOptionMultiple || gameType == domain.GameTypeFillInTheBlanks {
		errs = append(errs, optionErrors(it)...)
	}
	if gameType == domain.GameTypeMatching {
		errs = append(errs, matchingErrors(it)...)
	}
	return errs
}

func optionErrors(it semanticItem) []string {
	var errs []string

	ids := make(map[string]bool, len(it.Options))
	texts := make(map[string]bool, len(it.Options))
	for _, o := range it.Options {
		if ids[o.ID] {
			errs = append(errs, fmt.Sprintf("duplicate option id %q", o.ID))
		}
		ids[o.ID] = true

		if o.Text != strings.TrimSpace(o.Text) {
			errs = append(errs, fmt.Sprintf("option %q has leading or trailing whitespace", o.ID))
		}
		// Identical texts are indistinguishable on screen: only one id is accepted,
		// so a child picking the twin is marked wrong. Case matters here — spelling
		// questions legitimately offer "María" against "maría".
		key := strings.TrimSpace(o.Text)
		if texts[key] {
			errs = append(errs, fmt.Sprintf("duplicate option text %q", o.Text))
		}
		texts[key] = true
	}

	var correct []string
	if err := json.Unmarshal(it.CorrectAnswers, &correct); err != nil {
		return append(errs, "correctAnswers must be an array of option ids")
	}
	for _, c := range correct {
		if !ids[c] {
			errs = append(errs, fmt.Sprintf("correctAnswers references unknown option id %q", c))
		}
	}
	return errs
}

func matchingErrors(it semanticItem) []string {
	if it.Pairs == nil {
		return []string{"matching question is missing pairs"}
	}

	var errs []string
	var pairs []struct {
		LeftID  string `json:"leftId"`
		RightID string `json:"rightId"`
	}
	if err := json.Unmarshal(it.CorrectAnswers, &pairs); err != nil {
		return []string{"correctAnswers must be an array of {leftId, rightId}"}
	}

	leftIDs := make(map[string]bool, len(it.Pairs.Left))
	for _, l := range it.Pairs.Left {
		leftIDs[l.ID] = true
	}
	rightIDs := make(map[string]bool, len(it.Pairs.Right))
	for _, r := range it.Pairs.Right {
		rightIDs[r.ID] = true
	}

	if len(pairs) != len(it.Pairs.Left) {
		errs = append(errs, fmt.Sprintf("correctAnswers has %d pairs for %d left items", len(pairs), len(it.Pairs.Left)))
	}

	seenLeft := make(map[string]bool, len(pairs))
	seenRight := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		if !leftIDs[p.LeftID] {
			errs = append(errs, fmt.Sprintf("unknown leftId %q", p.LeftID))
		}
		if !rightIDs[p.RightID] {
			errs = append(errs, fmt.Sprintf("unknown rightId %q", p.RightID))
		}
		if seenLeft[p.LeftID] {
			errs = append(errs, fmt.Sprintf("leftId %q appears in more than one pair", p.LeftID))
		}
		seenLeft[p.LeftID] = true

		// The app frees a right item when it is reassigned, so two left items
		// pointing at one right id produce a question nobody can answer.
		if seenRight[p.RightID] {
			errs = append(errs, fmt.Sprintf("rightId %q is used by more than one pair; give each left item its own right item", p.RightID))
		}
		seenRight[p.RightID] = true
	}
	return errs
}

// ValidationErrors extracts a human-readable list of errors for the retry prompt.
func (v *QuestionValidator) ValidationErrors(gameType domain.GameType, raw []byte) string {
	if err := v.ValidateRaw(gameType, raw); err != nil {
		return err.Error()
	}
	return ""
}
