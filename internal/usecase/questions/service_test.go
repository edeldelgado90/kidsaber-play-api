package questions_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

// ─── Mocks ───────────────────────────────────────────────────────────────────

type mockGenerator struct {
	generateFunc func(ctx context.Context, params questions.GenerateParams) ([]domain.Question, error)
}

func (m *mockGenerator) Generate(ctx context.Context, params questions.GenerateParams) ([]domain.Question, error) {
	return m.generateFunc(ctx, params)
}

type mockRepo struct {
	findRandomFunc        func(ctx context.Context, params questions.FindParams, count int) ([]domain.Question, error)
	saveFunc              func(ctx context.Context, qs []domain.Question) error
	insertBatchFunc       func(ctx context.Context, qs []domain.Question) error
	countFunc             func(ctx context.Context, params questions.FindParams) (int, error)
	deleteMostUsedFunc    func(ctx context.Context, params questions.FindParams, limit int) error
	incrementUsageFunc    func(ctx context.Context, ids []string) error
}

func (m *mockRepo) FindRandom(ctx context.Context, params questions.FindParams, count int) ([]domain.Question, error) {
	return m.findRandomFunc(ctx, params, count)
}
func (m *mockRepo) Save(ctx context.Context, qs []domain.Question) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, qs)
	}
	return nil
}
func (m *mockRepo) InsertBatch(ctx context.Context, qs []domain.Question) error {
	if m.insertBatchFunc != nil {
		return m.insertBatchFunc(ctx, qs)
	}
	return nil
}
func (m *mockRepo) Count(ctx context.Context, params questions.FindParams) (int, error) {
	if m.countFunc != nil {
		return m.countFunc(ctx, params)
	}
	return 0, nil
}
func (m *mockRepo) DeleteMostUsed(ctx context.Context, params questions.FindParams, limit int) error {
	if m.deleteMostUsedFunc != nil {
		return m.deleteMostUsedFunc(ctx, params, limit)
	}
	return nil
}
func (m *mockRepo) IncrementUsageCount(ctx context.Context, ids []string) error {
	if m.incrementUsageFunc != nil {
		return m.incrementUsageFunc(ctx, ids)
	}
	return nil
}

type mockNotifier struct {
	alertCalled bool
	alertEvent  unotify.NotificationEvent
}

func (m *mockNotifier) Alert(_ context.Context, event unotify.NotificationEvent) error {
	m.alertCalled = true
	m.alertEvent = event
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func makeQuestion(id, gameType, subject string, grade int) domain.Question {
	ca, _ := json.Marshal([]string{"A"})
	return domain.Question{
		ID:             id,
		Type:           domain.GameType(gameType),
		Subject:        domain.Subject(subject),
		Grade:          grade,
		Topic:          "test_topic",
		Statement:      "Test question?",
		Options:        []domain.Option{{ID: "A", Text: "correct"}, {ID: "B", Text: "wrong"}},
		CorrectAnswers: json.RawMessage(ca),
		Meta:           domain.QuestionMeta{Difficulty: domain.DifficultyEasy, TimeLimitMs: 10000, Tags: []string{"test"}},
	}
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestGetQuestionsUseCase_QuickCalculation(t *testing.T) {
	calcGen := &mockGenerator{
		generateFunc: func(_ context.Context, params questions.GenerateParams) ([]domain.Question, error) {
			qs := make([]domain.Question, params.Count)
			for i := range qs {
				qs[i] = makeQuestion("id"+string(rune('0'+i)), "quick_calculation", "mathematics", params.Grade)
			}
			return qs, nil
		},
	}
	llmGen := &mockGenerator{
		generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
			t.Fatal("LLM generator should not be called for quick_calculation")
			return nil, nil
		},
	}
	repo := &mockRepo{
		findRandomFunc: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
			t.Fatal("repo should not be called for quick_calculation")
			return nil, nil
		},
	}
	notifier := &mockNotifier{}

	uc := questions.NewGetQuestionsUseCase(calcGen, llmGen, repo, notifier, newTestLogger())

	result, err := uc.Execute(context.Background(), questions.GetQuestionsParams{
		Subject: domain.SubjectMathematics,
		Grade:   3,
		Type:    domain.GameTypeQuickCalc,
		Count:   5,
	})

	require.NoError(t, err)
	assert.Len(t, result, 5)
}

func TestGetQuestionsUseCase_ServedFromPool(t *testing.T) {
	poolQuestions := make([]domain.Question, 10)
	for i := range poolQuestions {
		poolQuestions[i] = makeQuestion("pool-q-"+string(rune('0'+i)), "option_multiple", "mathematics", 3)
	}

	llmGen := &mockGenerator{
		generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
			t.Fatal("LLM generator should not be called when pool has enough questions")
			return nil, nil
		},
	}
	repo := &mockRepo{
		findRandomFunc: func(_ context.Context, _ questions.FindParams, count int) ([]domain.Question, error) {
			return poolQuestions[:count], nil
		},
	}
	notifier := &mockNotifier{}

	uc := questions.NewGetQuestionsUseCase(&mockGenerator{generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, nil
	}}, llmGen, repo, notifier, newTestLogger())

	result, err := uc.Execute(context.Background(), questions.GetQuestionsParams{
		Subject: domain.SubjectMathematics,
		Grade:   3,
		Type:    domain.GameTypeOptionMultiple,
		Count:   10,
	})

	require.NoError(t, err)
	assert.Len(t, result, 10)
}

func TestGetQuestionsUseCase_FallsBackToLLM_WhenPoolEmpty(t *testing.T) {
	llmQuestions := make([]domain.Question, 5)
	for i := range llmQuestions {
		llmQuestions[i] = makeQuestion("llm-q-"+string(rune('0'+i)), "option_multiple", "mathematics", 3)
	}

	llmCalled := false
	llmGen := &mockGenerator{
		generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
			llmCalled = true
			return llmQuestions, nil
		},
	}
	repo := &mockRepo{
		findRandomFunc: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
			return nil, nil // empty pool
		},
	}
	notifier := &mockNotifier{}

	uc := questions.NewGetQuestionsUseCase(&mockGenerator{generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, nil
	}}, llmGen, repo, notifier, newTestLogger())

	result, err := uc.Execute(context.Background(), questions.GetQuestionsParams{
		Subject: domain.SubjectMathematics,
		Grade:   3,
		Type:    domain.GameTypeOptionMultiple,
		Count:   5,
	})

	require.NoError(t, err)
	assert.Len(t, result, 5)
	assert.True(t, llmCalled)
}

func TestGetQuestionsUseCase_RetriesDBBeforeLLM(t *testing.T) {
	dbCallCount := 0
	llmCalled := false

	llmQuestions := make([]domain.Question, 10)
	for i := range llmQuestions {
		llmQuestions[i] = makeQuestion("llm-q-"+string(rune('0'+i)), "option_multiple", "mathematics", 3)
	}

	llmGen := &mockGenerator{
		generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
			llmCalled = true
			return llmQuestions, nil
		},
	}
	repo := &mockRepo{
		findRandomFunc: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
			dbCallCount++
			return nil, nil // always empty pool
		},
	}
	notifier := &mockNotifier{}

	uc := questions.NewGetQuestionsUseCase(
		&mockGenerator{generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
			return nil, nil
		}},
		llmGen, repo, notifier, newTestLogger(),
	)

	result, err := uc.Execute(context.Background(), questions.GetQuestionsParams{
		Subject: domain.SubjectMathematics,
		Grade:   3,
		Type:    domain.GameTypeOptionMultiple,
		Count:   10,
	})

	require.NoError(t, err)
	assert.Len(t, result, 10)
	assert.Equal(t, 3, dbCallCount, "expected exactly 3 DB attempts before LLM fallback")
	assert.True(t, llmCalled)
}

func TestGetQuestionsUseCase_ServesFromPoolOnSecondDBAttempt(t *testing.T) {
	dbCallCount := 0
	poolQuestions := make([]domain.Question, 10)
	for i := range poolQuestions {
		poolQuestions[i] = makeQuestion("pool-q-"+string(rune('0'+i)), "option_multiple", "mathematics", 3)
	}

	llmGen := &mockGenerator{
		generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
			t.Fatal("LLM generator should not be called when pool succeeds on retry")
			return nil, nil
		},
	}
	repo := &mockRepo{
		findRandomFunc: func(_ context.Context, _ questions.FindParams, count int) ([]domain.Question, error) {
			dbCallCount++
			if dbCallCount < 2 {
				return nil, nil // first attempt returns empty
			}
			return poolQuestions[:count], nil // second attempt succeeds
		},
	}
	notifier := &mockNotifier{}

	uc := questions.NewGetQuestionsUseCase(
		&mockGenerator{generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
			return nil, nil
		}},
		llmGen, repo, notifier, newTestLogger(),
	)

	result, err := uc.Execute(context.Background(), questions.GetQuestionsParams{
		Subject: domain.SubjectMathematics,
		Grade:   3,
		Type:    domain.GameTypeOptionMultiple,
		Count:   10,
	})

	require.NoError(t, err)
	assert.Len(t, result, 10)
	assert.Equal(t, 2, dbCallCount, "expected 2 DB attempts before serving from pool")
}

func TestGetQuestionsUseCase_LLMFails_ReturnsError(t *testing.T) {
	llmGen := &mockGenerator{
		generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
			return nil, domain.ErrNoValidQuestions
		},
	}
	repo := &mockRepo{
		findRandomFunc: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
			return nil, nil
		},
	}
	notifier := &mockNotifier{}

	uc := questions.NewGetQuestionsUseCase(&mockGenerator{generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, nil
	}}, llmGen, repo, notifier, newTestLogger())

	_, err := uc.Execute(context.Background(), questions.GetQuestionsParams{
		Subject: domain.SubjectLanguage,
		Grade:   4,
		Type:    domain.GameTypeFillInTheBlanks,
		Count:   10,
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNoValidQuestions))
}

func TestGetQuestionsUseCase_DefaultsCountTo10(t *testing.T) {
	var calledWithCount int
	llmGen := &mockGenerator{
		generateFunc: func(_ context.Context, params questions.GenerateParams) ([]domain.Question, error) {
			calledWithCount = params.Count
			qs := make([]domain.Question, params.Count)
			return qs, nil
		},
	}
	repo := &mockRepo{
		findRandomFunc: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
			return nil, nil
		},
	}
	notifier := &mockNotifier{}

	uc := questions.NewGetQuestionsUseCase(&mockGenerator{generateFunc: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, nil
	}}, llmGen, repo, notifier, newTestLogger())

	_, err := uc.Execute(context.Background(), questions.GetQuestionsParams{
		Subject: domain.SubjectEnglish,
		Grade:   2,
		Type:    domain.GameTypeMatching,
		Count:   0, // should default to 10
	})

	require.NoError(t, err)
	assert.Equal(t, 10, calledWithCount)
}
