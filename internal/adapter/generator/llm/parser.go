package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// ParseQuestions parses the raw LLM response text into a slice of domain.Question.
// The LLM is instructed to return a JSON array starting with "[".
// This function strips any markdown fences that models occasionally emit.
func ParseQuestions(responseText string) ([]byte, []domain.Question, error) {
	raw := cleanLLMResponse(responseText)

	var questions []domain.Question
	if err := json.Unmarshal(raw, &questions); err != nil {
		return raw, nil, fmt.Errorf("parsing LLM JSON: %w", err)
	}

	return raw, questions, nil
}

// cleanLLMResponse strips markdown code fences and trims whitespace.
// Models sometimes wrap JSON in ```json ... ``` despite explicit instructions.
func cleanLLMResponse(text string) []byte {
	text = strings.TrimSpace(text)

	// Strip markdown fences: ```json ... ``` or ``` ... ```
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	// Find the first "[" to handle any preamble text
	idx := strings.Index(text, "[")
	if idx > 0 {
		text = text[idx:]
	}

	// Find the last "]" to handle any postamble text
	lastIdx := strings.LastIndex(text, "]")
	if lastIdx != -1 && lastIdx < len(text)-1 {
		text = text[:lastIdx+1]
	}

	return []byte(text)
}
