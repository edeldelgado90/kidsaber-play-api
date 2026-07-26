package domain_test

import (
	"testing"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

func TestIsValidSubject(t *testing.T) {
	cases := []struct {
		input domain.Subject
		want  bool
	}{
		{domain.SubjectMathematics, true},
		{domain.SubjectLanguage, true},
		{domain.SubjectEnglish, true},
		{domain.SubjectScience, true},
		{"history", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := domain.IsValidSubject(tc.input); got != tc.want {
			t.Errorf("IsValidSubject(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsValidGameType(t *testing.T) {
	cases := []struct {
		input domain.GameType
		want  bool
	}{
		{domain.GameTypeOptionMultiple, true},
		{domain.GameTypeFillInTheBlanks, true},
		{domain.GameTypeMatching, true},
		{domain.GameTypeQuickCalc, true},
		{"unknown", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := domain.IsValidGameType(tc.input); got != tc.want {
			t.Errorf("IsValidGameType(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsValidGrade(t *testing.T) {
	cases := []struct {
		input int
		want  bool
	}{
		{1, true},
		{3, true},
		{6, true},
		{0, false},
		{7, false},
		{-1, false},
	}
	for _, tc := range cases {
		if got := domain.IsValidGrade(tc.input); got != tc.want {
			t.Errorf("IsValidGrade(%d) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsValidCount(t *testing.T) {
	cases := []struct {
		input int
		want  bool
	}{
		{1, true},
		{10, true},
		{30, true},
		{0, false},
		{31, false},
		{-5, false},
	}
	for _, tc := range cases {
		if got := domain.IsValidCount(tc.input); got != tc.want {
			t.Errorf("IsValidCount(%d) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsLLMGameType(t *testing.T) {
	cases := []struct {
		input domain.GameType
		want  bool
	}{
		{domain.GameTypeOptionMultiple, true},
		{domain.GameTypeFillInTheBlanks, true},
		{domain.GameTypeMatching, true},
		{domain.GameTypeQuickCalc, false},
		{"unknown", false},
	}
	for _, tc := range cases {
		if got := domain.IsLLMGameType(tc.input); got != tc.want {
			t.Errorf("IsLLMGameType(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestSubjectSpanish(t *testing.T) {
	cases := []struct {
		input domain.Subject
		want  string
	}{
		{domain.SubjectMathematics, "Matemáticas"},
		{domain.SubjectLanguage, "Lengua Castellana y Literatura"},
		{domain.SubjectEnglish, "Inglés"},
		{domain.SubjectScience, "Conocimiento del Medio Natural, Social y Cultural"},
		{"other", "other"},
	}
	for _, tc := range cases {
		if got := domain.SubjectSpanish(tc.input); got != tc.want {
			t.Errorf("SubjectSpanish(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
