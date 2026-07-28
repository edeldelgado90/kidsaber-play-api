// Command audit inspects the question_bank for content and rendering defects.
//
// It is read-only: it reports findings and never writes. Run it against any
// environment by pointing DATABASE_URL at the target database.
//
//	DATABASE_URL="postgres://..." go run ./cmd/audit
//	DATABASE_URL="postgres://..." go run ./cmd/audit -v   # list every affected row
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// blankMarker is the exact placeholder the app splits on in
// FillBlankStatement.tsx. Any other run of underscores renders literally.
const blankMarker = "____"

var underscoreRun = regexp.MustCompile(`_+`)

type row struct {
	ID      string
	Subject string
	Grade   int
	Type    string
	Payload payload
}

type payload struct {
	Statement string `json:"statement"`
	Options   []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"options"`
	Pairs *struct {
		Left  []item `json:"left"`
		Right []item `json:"right"`
	} `json:"pairs"`
	CorrectAnswers json.RawMessage `json:"correctAnswers"`
}

type item struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type matchPair struct {
	LeftID  string `json:"leftId"`
	RightID string `json:"rightId"`
}

type finding struct {
	code string
	row  row
	note string
}

func main() {
	verbose := flag.Bool("v", false, "list every affected row")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	rows, err := loadRows(ctx, conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Preguntas analizadas: %d\n\n", len(rows))

	findings := audit(rows)
	report(findings, *verbose)
}

func loadRows(ctx context.Context, conn *pgx.Conn) ([]row, error) {
	q := `SELECT id, subject, grade, type, payload FROM question_bank ORDER BY grade, subject, type`
	res, err := conn.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	var out []row
	for res.Next() {
		var r row
		var raw []byte
		if err := res.Scan(&r.ID, &r.Subject, &r.Grade, &r.Type, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &r.Payload); err != nil {
			return nil, fmt.Errorf("row %s: %w", r.ID, err)
		}
		out = append(out, r)
	}
	return out, res.Err()
}

func audit(rows []row) []finding {
	var fs []finding
	add := func(code string, r row, note string) {
		fs = append(fs, finding{code: code, row: r, note: note})
	}

	seen := map[string][]row{}

	for _, r := range rows {
		p := r.Payload
		st := p.Statement

		key := fmt.Sprintf("%d|%s|%s|%s", r.Grade, r.Subject, r.Type, strings.ToLower(strings.TrimSpace(st)))
		seen[key] = append(seen[key], r)

		switch r.Type {
		case "fill_in_the_blanks":
			runs := underscoreRun.FindAllString(st, -1)
			switch {
			case len(runs) == 0:
				add("fib_sin_hueco", r, "el hueco se pinta al final del enunciado")
			case len(runs) > 1:
				add("fib_multiples_huecos", r, fmt.Sprintf("%d huecos; la app solo pinta el primero y trunca el resto", len(runs)))
			case runs[0] != blankMarker:
				add("fib_marcador_incorrecto", r, fmt.Sprintf("%q en vez de %q: se ven guiones bajos sueltos", runs[0], blankMarker))
			}
			if n := len(p.Options); n != 3 {
				add("fib_numero_opciones", r, fmt.Sprintf("%d opciones (deben ser 3)", n))
			}
			checkOptions(r, add)

		case "option_multiple":
			if n := len(p.Options); n != 4 {
				add("om_numero_opciones", r, fmt.Sprintf("%d opciones (deben ser 4)", n))
			}
			checkOptions(r, add)

		case "matching":
			checkMatching(r, add)
		}

		for _, s := range allText(p) {
			if s != strings.TrimSpace(s) {
				add("espacios_sobrantes", r, fmt.Sprintf("%q", s))
			}
			if strings.Contains(s, "  ") {
				add("espacio_doble", r, fmt.Sprintf("%q", s))
			}
			if strings.ContainsAny(s, "\n\t") {
				add("salto_de_linea", r, fmt.Sprintf("%q", s))
			}
		}
	}

	for _, group := range seen {
		if len(group) > 1 {
			add("enunciado_duplicado", group[0], fmt.Sprintf("%d copias", len(group)))
		}
	}

	return fs
}

// checkOptions validates option IDs, the correct answer reference and looks for
// options a child cannot tell apart.
func checkOptions(r row, add func(string, row, string)) {
	p := r.Payload
	if len(p.Options) == 0 {
		return
	}

	ids := map[string]bool{}
	for _, o := range p.Options {
		if ids[o.ID] {
			add("ids_duplicados", r, o.ID)
		}
		ids[o.ID] = true
	}

	var correct []string
	if err := json.Unmarshal(p.CorrectAnswers, &correct); err != nil || len(correct) != 1 {
		add("correctAnswers_invalido", r, string(p.CorrectAnswers))
		return
	}
	if !ids[correct[0]] {
		add("respuesta_inexistente", r, fmt.Sprintf("correctAnswers=%v no está entre las opciones", correct))
	}

	// Two options with identical text: only one ID is accepted, so a child who
	// picks the visually identical twin is marked wrong. The comparison is
	// case-sensitive on purpose — spelling questions ask the child to tell
	// "María" from "maría", and those options are distinguishable on screen.
	byText := map[string][]string{}
	for _, o := range p.Options {
		t := strings.TrimSpace(o.Text)
		byText[t] = append(byText[t], o.ID)
	}
	for text, dupIDs := range byText {
		if len(dupIDs) > 1 {
			note := fmt.Sprintf("%q aparece en %v", text, dupIDs)
			for _, id := range dupIDs {
				if id == correct[0] {
					note += " — una de ellas es la correcta: imposible distinguirlas"
					break
				}
			}
			add("opciones_identicas", r, note)
		}
	}
}

// checkMatching looks for pairings the MatchingColumn UI cannot express.
func checkMatching(r row, add func(string, row, string)) {
	p := r.Payload
	if p.Pairs == nil {
		add("matching_sin_pairs", r, "")
		return
	}

	var pairs []matchPair
	if err := json.Unmarshal(p.CorrectAnswers, &pairs); err != nil {
		add("correctAnswers_invalido", r, string(p.CorrectAnswers))
		return
	}

	leftIDs := map[string]bool{}
	for _, l := range p.Pairs.Left {
		leftIDs[l.ID] = true
	}
	rightText := map[string]string{}
	for _, x := range p.Pairs.Right {
		rightText[x.ID] = x.Text
	}

	if len(pairs) != len(p.Pairs.Left) {
		add("matching_pares_incompletos", r, fmt.Sprintf("%d pares para %d elementos", len(pairs), len(p.Pairs.Left)))
	}

	for _, pr := range pairs {
		if !leftIDs[pr.LeftID] {
			add("matching_id_invalido", r, "leftId "+pr.LeftID)
		}
		if _, ok := rightText[pr.RightID]; !ok {
			add("matching_id_invalido", r, "rightId "+pr.RightID)
		}
	}

	// The UI frees a right item when it is reassigned (MatchingColumn.tsx), so
	// two left items mapped to one right ID can never both be selected.
	usedRight := map[string]int{}
	for _, pr := range pairs {
		usedRight[pr.RightID]++
	}
	for id, n := range usedRight {
		if n > 1 {
			add("matching_imposible", r, fmt.Sprintf("%d elementos apuntan a %s (%q); la UI solo permite uno", n, id, rightText[id]))
		}
	}

	// Distinct IDs carrying the same label are indistinguishable on screen.
	byLabel := map[string][]string{}
	for _, x := range p.Pairs.Right {
		byLabel[strings.TrimSpace(x.Text)] = append(byLabel[strings.TrimSpace(x.Text)], x.ID)
	}
	for label, dupIDs := range byLabel {
		if len(dupIDs) > 1 {
			add("matching_ambiguo", r, fmt.Sprintf("%q se repite en la columna B (%v)", label, dupIDs))
		}
	}
}

func allText(p payload) []string {
	out := []string{p.Statement}
	for _, o := range p.Options {
		out = append(out, o.Text)
	}
	if p.Pairs != nil {
		for _, x := range p.Pairs.Left {
			out = append(out, x.Text)
		}
		for _, x := range p.Pairs.Right {
			out = append(out, x.Text)
		}
	}
	return out
}

func report(fs []finding, verbose bool) {
	byCode := map[string][]finding{}
	for _, f := range fs {
		byCode[f.code] = append(byCode[f.code], f)
	}

	codes := make([]string, 0, len(byCode))
	for c := range byCode {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool {
		return len(byCode[codes[i]]) > len(byCode[codes[j]])
	})

	if len(codes) == 0 {
		fmt.Println("Sin incidencias.")
		return
	}

	fmt.Println("=== RESUMEN ===")
	for _, c := range codes {
		fmt.Printf("%6d  %s\n", len(byCode[c]), c)
	}

	if !verbose {
		fmt.Println("\n(usa -v para el detalle por fila)")
		return
	}

	for _, c := range codes {
		fmt.Printf("\n=== %s (%d) ===\n", c, len(byCode[c]))
		for _, f := range byCode[c] {
			fmt.Printf("  %s  G%d %-12s %-18s | %s | %s\n",
				f.row.ID, f.row.Grade, f.row.Subject, f.row.Type,
				truncate(f.row.Payload.Statement, 70), f.note)
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
