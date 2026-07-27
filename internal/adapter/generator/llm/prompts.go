package llm

import (
	"bytes"
	"fmt"
	"sync"
	"text/template"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// PromptData is the data injected into LLM prompt templates.
type PromptData struct {
	SubjectSpanish string // e.g. "Matemáticas"
	Subject        string // e.g. "mathematics"
	Grade          int    // 1–6
	AgeMin         int    // Grade + 5
	AgeMax         int    // Grade + 6
	Topic          string // e.g. "multiplication_tables_2_to_5"
	Count          int    // number of questions to generate
}

// RetryPromptData is the data injected into the retry correction prefix.
type RetryPromptData struct {
	ValidationErrors string
	Count            int
}

// Templates are parsed once at startup.
var (
	templateOnce     sync.Once
	promptTemplates  map[domain.GameType]*template.Template
	retryTemplate    *template.Template
	templateParseErr error
)

// Raw prompt templates as backtick string literals — binary stays self-contained.
const optionMultiplePromptTmpl = `Eres un generador de preguntas educativas para niños de Primaria en España (currículo LOMLOE).

Genera exactamente {{.Count}} preguntas de opción múltiple para:
- Asignatura: {{.SubjectSpanish}}
- Curso: {{.Grade}}º de Primaria (niños de {{.AgeMin}}–{{.AgeMax}} años)
- Tema: {{.Topic}}

REGLAS OBLIGATORIAS:
- 4 opciones por pregunta (ids "A", "B", "C", "D"); orden aleatorio.
- Una única respuesta correcta por pregunta.
- Lenguaje sencillo, claro y motivador para niños de {{.AgeMin}}–{{.AgeMax}} años.
- Las opciones incorrectas deben ser plausibles pero claramente distintas de la correcta.
- No repitas preguntas ni opciones idénticas.

FORMATO DE RESPUESTA: responde ÚNICAMENTE con un array JSON válido. Sin markdown, sin explicaciones.

Schema de cada objeto del array:
{
  "type": "option_multiple",
  "subject": "{{.Subject}}",
  "grade": {{.Grade}},
  "topic": "{{.Topic}}",
  "statement": "<enunciado de la pregunta>",
  "options": [
    { "id": "A", "text": "<texto opción A>" },
    { "id": "B", "text": "<texto opción B>" },
    { "id": "C", "text": "<texto opción C>" },
    { "id": "D", "text": "<texto opción D>" }
  ],
  "correctAnswers": ["<id de la opción correcta>"],
  "meta": {
    "difficulty": "<easy|medium|hard>",
    "timeLimitMs": <número entero, ms recomendados>,
    "tags": ["<tag1>", "<tag2>"]
  }
}

Ejemplo de objeto válido:
{
  "type": "option_multiple",
  "subject": "mathematics",
  "grade": 3,
  "topic": "multiplication_tables_complete",
  "statement": "¿Cuánto es 4 × 6?",
  "options": [
    { "id": "A", "text": "20" },
    { "id": "B", "text": "24" },
    { "id": "C", "text": "26" },
    { "id": "D", "text": "16" }
  ],
  "correctAnswers": ["B"],
  "meta": { "difficulty": "easy", "timeLimitMs": 15000, "tags": ["multiplication", "tables"] }
}

Responde SOLO con el array JSON. Empieza directamente con "[".`

const fillInTheBlanksPromptTmpl = `Eres un generador de preguntas educativas para niños de Primaria en España (currículo LOMLOE).

Genera exactamente {{.Count}} preguntas de completar huecos para:
- Asignatura: {{.SubjectSpanish}}
- Curso: {{.Grade}}º de Primaria (niños de {{.AgeMin}}–{{.AgeMax}} años)
- Tema: {{.Topic}}

REGLAS OBLIGATORIAS:
- El enunciado (statement) debe contener exactamente un hueco marcado con "____".
- Incluir exactamente 3 opciones de selección (ids "A", "B", "C"); una es la correcta.
- Lenguaje adecuado para niños de {{.AgeMin}}–{{.AgeMax}} años.
- Las opciones incorrectas deben ser gramaticalmente plausibles pero semánticamente incorrectas.
- No repitas frases ni opciones idénticas entre preguntas.

FORMATO DE RESPUESTA: responde ÚNICAMENTE con un array JSON válido. Sin markdown, sin explicaciones.

Schema de cada objeto del array:
{
  "type": "fill_in_the_blanks",
  "subject": "{{.Subject}}",
  "grade": {{.Grade}},
  "topic": "{{.Topic}}",
  "statement": "<frase con ____ marcando el hueco>",
  "options": [
    { "id": "A", "text": "<opción A>" },
    { "id": "B", "text": "<opción B>" },
    { "id": "C", "text": "<opción C>" }
  ],
  "correctAnswers": ["<id de la opción correcta>"],
  "meta": {
    "difficulty": "<easy|medium|hard>",
    "timeLimitMs": <número entero, ms>,
    "tags": ["<tag1>", "<tag2>"]
  }
}

Ejemplo de objeto válido:
{
  "type": "fill_in_the_blanks",
  "subject": "language",
  "grade": 3,
  "topic": "verb_conjugation_present_perfect_future",
  "statement": "Mi hermano ____ al colegio todos los días.",
  "options": [
    { "id": "A", "text": "va" },
    { "id": "B", "text": "voy" },
    { "id": "C", "text": "fui" }
  ],
  "correctAnswers": ["A"],
  "meta": { "difficulty": "easy", "timeLimitMs": 20000, "tags": ["verb", "present_tense"] }
}

Responde SOLO con el array JSON. Empieza directamente con "[".`

const matchingPromptTmpl = `Eres un generador de preguntas educativas para niños de Primaria en España (currículo LOMLOE).

Genera exactamente {{.Count}} preguntas de emparejar para:
- Asignatura: {{.SubjectSpanish}}
- Curso: {{.Grade}}º de Primaria (niños de {{.AgeMin}}–{{.AgeMax}} años)
- Tema: {{.Topic}}

REGLAS OBLIGATORIAS:
- Cada pregunta tiene entre 3 y 4 parejas (columna izquierda "L1"–"L4", columna derecha "R1"–"R4").
- Cada elemento izquierdo tiene exactamente un par correcto en la derecha.
- Los elementos de la columna derecha deben aparecer en orden aleatorio (no alineados con los de la izquierda).
- Lenguaje adecuado para niños de {{.AgeMin}}–{{.AgeMax}} años.
- No repitas conjuntos de parejas idénticos entre preguntas.

FORMATO DE RESPUESTA: responde ÚNICAMENTE con un array JSON válido. Sin markdown, sin explicaciones.

Schema de cada objeto del array:
{
  "type": "matching",
  "subject": "{{.Subject}}",
  "grade": {{.Grade}},
  "topic": "{{.Topic}}",
  "statement": "<instrucción para emparejar>",
  "pairs": {
    "left":  [ { "id": "L1", "text": "<texto>" }, ... ],
    "right": [ { "id": "R1", "text": "<texto>" }, ... ]
  },
  "correctAnswers": [
    { "leftId": "L1", "rightId": "<Rx correspondiente>" },
    ...
  ],
  "meta": {
    "difficulty": "<easy|medium|hard>",
    "timeLimitMs": <número entero, ms>,
    "tags": ["<tag1>", "<tag2>"]
  }
}

Ejemplo de objeto válido:
{
  "type": "matching",
  "subject": "science",
  "grade": 3,
  "topic": "ecosystems",
  "statement": "Une cada animal con su tipo.",
  "pairs": {
    "left":  [ { "id": "L1", "text": "Perro" }, { "id": "L2", "text": "Águila" }, { "id": "L3", "text": "Salmón" } ],
    "right": [ { "id": "R1", "text": "Ave" },   { "id": "R2", "text": "Pez" },    { "id": "R3", "text": "Mamífero" } ]
  },
  "correctAnswers": [
    { "leftId": "L1", "rightId": "R3" },
    { "leftId": "L2", "rightId": "R1" },
    { "leftId": "L3", "rightId": "R2" }
  ],
  "meta": { "difficulty": "medium", "timeLimitMs": 30000, "tags": ["animals", "classification"] }
}

Responde SOLO con el array JSON. Empieza directamente con "[".`

const retryPromptTmpl = `CORRECCIÓN NECESARIA: Tu respuesta anterior no cumplió el formato requerido.
Errores detectados: {{.ValidationErrors}}
Vuelve a generar las {{.Count}} preguntas siguiendo el schema EXACTAMENTE.
Responde SOLO con el array JSON. Empieza con "[" y termina con "]".
---
`

// initTemplates parses all templates exactly once.
func initTemplates() {
	templateOnce.Do(func() {
		promptTemplates = make(map[domain.GameType]*template.Template)

		defs := map[domain.GameType]string{
			domain.GameTypeOptionMultiple:  optionMultiplePromptTmpl,
			domain.GameTypeFillInTheBlanks: fillInTheBlanksPromptTmpl,
			domain.GameTypeMatching:        matchingPromptTmpl,
		}

		for gt, def := range defs {
			t, err := template.New(string(gt)).Parse(def)
			if err != nil {
				templateParseErr = fmt.Errorf("parsing template for %s: %w", gt, err)
				return
			}
			promptTemplates[gt] = t
		}

		rt, err := template.New("retry").Parse(retryPromptTmpl)
		if err != nil {
			templateParseErr = fmt.Errorf("parsing retry template: %w", err)
			return
		}
		retryTemplate = rt
	})
}

// BuildPrompt renders the prompt template for the given game type and data.
func BuildPrompt(gameType domain.GameType, data PromptData) (string, error) {
	initTemplates()
	if templateParseErr != nil {
		return "", templateParseErr
	}

	tmpl, ok := promptTemplates[gameType]
	if !ok {
		return "", fmt.Errorf("no prompt template for game type: %s", gameType)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing prompt template: %w", err)
	}
	return buf.String(), nil
}

// BuildRetryPrompt prepends the correction header to the original prompt.
func BuildRetryPrompt(originalPrompt, validationErrors string, count int) (string, error) {
	initTemplates()
	if templateParseErr != nil {
		return "", templateParseErr
	}

	var buf bytes.Buffer
	if err := retryTemplate.Execute(&buf, RetryPromptData{
		ValidationErrors: validationErrors,
		Count:            count,
	}); err != nil {
		return "", fmt.Errorf("executing retry template: %w", err)
	}

	return buf.String() + originalPrompt, nil
}
