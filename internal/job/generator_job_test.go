package job

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

// ─── mocks ───────────────────────────────────────────────────────────────────

type mockGenerator struct {
	fn func(ctx context.Context, params questions.GenerateParams) ([]domain.Question, error)
}

func (m *mockGenerator) Generate(ctx context.Context, params questions.GenerateParams) ([]domain.Question, error) {
	return m.fn(ctx, params)
}

type mockQRepo struct {
	insertBatchFn    func(ctx context.Context, qs []domain.Question) error
	countFn          func(ctx context.Context, params questions.FindParams) (int, error)
	deleteMostUsedFn func(ctx context.Context, params questions.FindParams, limit int) error
}

func (m *mockQRepo) FindRandom(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
	return nil, nil
}
func (m *mockQRepo) Save(_ context.Context, _ []domain.Question) error { return nil }
func (m *mockQRepo) InsertBatch(ctx context.Context, qs []domain.Question) error {
	if m.insertBatchFn != nil {
		return m.insertBatchFn(ctx, qs)
	}
	return nil
}
func (m *mockQRepo) Count(ctx context.Context, params questions.FindParams) (int, error) {
	if m.countFn != nil {
		return m.countFn(ctx, params)
	}
	return 0, nil
}
func (m *mockQRepo) DeleteMostUsed(ctx context.Context, params questions.FindParams, limit int) error {
	if m.deleteMostUsedFn != nil {
		return m.deleteMostUsedFn(ctx, params, limit)
	}
	return nil
}
func (m *mockQRepo) IncrementUsageCount(_ context.Context, _ []string) error { return nil }

type mockJobRepo struct {
	insertFn func(ctx context.Context, run *domain.JobRun) error
	updateFn func(ctx context.Context, run *domain.JobRun) error
	savedRun *domain.JobRun
}

func (m *mockJobRepo) Insert(ctx context.Context, run *domain.JobRun) error {
	m.savedRun = run
	if m.insertFn != nil {
		return m.insertFn(ctx, run)
	}
	return nil
}
func (m *mockJobRepo) Update(ctx context.Context, run *domain.JobRun) error {
	m.savedRun = run
	if m.updateFn != nil {
		return m.updateFn(ctx, run)
	}
	return nil
}
func (m *mockJobRepo) FindRecent(_ context.Context, _ int) ([]domain.JobRun, error) {
	return nil, nil
}

type mockNotifier struct {
	alertErr error
	called   bool
}

func (m *mockNotifier) Alert(_ context.Context, _ unotify.NotificationEvent) error {
	m.called = true
	return m.alertErr
}

type mockPicker struct {
	fn func(subject domain.Subject, grade int, gameType domain.GameType) (string, error)
}

func (m *mockPicker) Pick(subject domain.Subject, grade int, gameType domain.GameType) (string, error) {
	return m.fn(subject, grade, gameType)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func defaultPicker() *mockPicker {
	return &mockPicker{fn: func(subject domain.Subject, grade int, gameType domain.GameType) (string, error) {
		return "general", nil
	}}
}

func defaultConfig() Config {
	return Config{
		BatchSize:         2,
		MaxPerCombination: 10,
		CombinationDelay:  0,
	}
}

// singleCombination overrides the package-level combinations var for the duration of the test.
func singleCombination(t *testing.T) {
	t.Helper()
	orig := combinations
	combinations = []Combination{
		{Subject: domain.SubjectMathematics, Grade: 3, GameType: domain.GameTypeOptionMultiple},
	}
	t.Cleanup(func() { combinations = orig })
}

func twoCombinations(t *testing.T) {
	t.Helper()
	orig := combinations
	combinations = []Combination{
		{Subject: domain.SubjectMathematics, Grade: 3, GameType: domain.GameTypeOptionMultiple},
		{Subject: domain.SubjectLanguage, Grade: 2, GameType: domain.GameTypeFillInTheBlanks},
	}
	t.Cleanup(func() { combinations = orig })
}

func makeQuestion() domain.Question {
	return domain.Question{
		ID:      "q1",
		Type:    domain.GameTypeOptionMultiple,
		Subject: domain.SubjectMathematics,
		Grade:   3,
	}
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestRun_AllSuccess_NoExcess(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return []domain.Question{makeQuestion()}, nil
	}}
	repo := &mockQRepo{countFn: func(_ context.Context, _ questions.FindParams) (int, error) {
		return 5, nil // well below MaxPerCombination=10
	}}
	jobRepo := &mockJobRepo{}
	notifier := &mockNotifier{}

	job := NewQuestionGeneratorJob(gen, repo, jobRepo, notifier, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusSuccess, jobRepo.savedRun.Status)
	assert.Equal(t, 0, jobRepo.savedRun.CombinationsFailed)
	assert.Equal(t, 1, jobRepo.savedRun.CombinationsDone)
	assert.False(t, notifier.called)
}

func TestRun_AllSuccess_WithExcess(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return []domain.Question{makeQuestion(), makeQuestion()}, nil
	}}
	deletedCount := 0
	repo := &mockQRepo{
		countFn: func(_ context.Context, _ questions.FindParams) (int, error) {
			return 15, nil // 5 above MaxPerCombination=10
		},
		deleteMostUsedFn: func(_ context.Context, _ questions.FindParams, limit int) error {
			deletedCount = limit
			return nil
		},
	}
	jobRepo := &mockJobRepo{}

	job := NewQuestionGeneratorJob(gen, repo, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 5, deletedCount)
	assert.Equal(t, 5, jobRepo.savedRun.QuestionsDeleted)
}

func TestRun_DeleteMostUsedFails_JobStillSucceeds(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return []domain.Question{makeQuestion()}, nil
	}}
	repo := &mockQRepo{
		countFn: func(_ context.Context, _ questions.FindParams) (int, error) {
			return 15, nil
		},
		deleteMostUsedFn: func(_ context.Context, _ questions.FindParams, _ int) error {
			return errors.New("delete failed")
		},
	}
	jobRepo := &mockJobRepo{}

	job := NewQuestionGeneratorJob(gen, repo, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusSuccess, jobRepo.savedRun.Status)
}

func TestRun_CountError_ExcessDeletionSkipped(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return []domain.Question{makeQuestion()}, nil
	}}
	deleteWasCalled := false
	repo := &mockQRepo{
		countFn: func(_ context.Context, _ questions.FindParams) (int, error) {
			return 0, errors.New("count error")
		},
		deleteMostUsedFn: func(_ context.Context, _ questions.FindParams, _ int) error {
			deleteWasCalled = true
			return nil
		},
	}
	jobRepo := &mockJobRepo{}

	job := NewQuestionGeneratorJob(gen, repo, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	require.NoError(t, err)
	assert.False(t, deleteWasCalled)
}

func TestRun_GeneratorFails(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, errors.New("llm error")
	}}
	jobRepo := &mockJobRepo{}
	notifier := &mockNotifier{}

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, notifier, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	assert.Error(t, err)
	assert.Equal(t, domain.JobStatusFailed, jobRepo.savedRun.Status)
	assert.Equal(t, 1, jobRepo.savedRun.CombinationsFailed)
	assert.True(t, notifier.called)
	require.Len(t, jobRepo.savedRun.ErrorDetails, 1)
	assert.Equal(t, "mathematics", jobRepo.savedRun.ErrorDetails[0].Subject)
	assert.Equal(t, 3, jobRepo.savedRun.ErrorDetails[0].Grade)
}

func TestRun_InsertBatchFails(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return []domain.Question{makeQuestion()}, nil
	}}
	repo := &mockQRepo{
		insertBatchFn: func(_ context.Context, _ []domain.Question) error {
			return errors.New("insert failed")
		},
	}
	jobRepo := &mockJobRepo{}
	notifier := &mockNotifier{}

	job := NewQuestionGeneratorJob(gen, repo, jobRepo, notifier, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	assert.Error(t, err)
	assert.Equal(t, 1, jobRepo.savedRun.CombinationsFailed)
	assert.True(t, notifier.called)
}

func TestRun_AllCombinationsFail_StatusFailed(t *testing.T) {
	twoCombinations(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, errors.New("always fails")
	}}
	jobRepo := &mockJobRepo{}

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	_ = job.Run(context.Background())

	assert.Equal(t, domain.JobStatusFailed, jobRepo.savedRun.Status)
	assert.Equal(t, 2, jobRepo.savedRun.CombinationsFailed)
}

func TestRun_Mixed_StatusPartial(t *testing.T) {
	twoCombinations(t)

	callCount := 0
	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		callCount++
		if callCount == 1 {
			return nil, errors.New("first fails")
		}
		return []domain.Question{makeQuestion()}, nil
	}}
	jobRepo := &mockJobRepo{}

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	_ = job.Run(context.Background())

	assert.Equal(t, domain.JobStatusPartial, jobRepo.savedRun.Status)
	assert.Equal(t, 1, jobRepo.savedRun.CombinationsFailed)
	assert.Equal(t, 1, jobRepo.savedRun.CombinationsDone)
}

func TestRun_JobRepoInsertFails_ExecutionContinues(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return []domain.Question{makeQuestion()}, nil
	}}
	jobRepo := &mockJobRepo{
		insertFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("insert failed")
		},
	}

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	require.NoError(t, err)
	// Execution should have completed successfully despite Insert failure
	assert.Equal(t, domain.JobStatusSuccess, jobRepo.savedRun.Status)
}

func TestRun_JobRepoUpdateFails_DoesNotMaskReturn(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return []domain.Question{makeQuestion()}, nil
	}}
	jobRepo := &mockJobRepo{
		updateFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("update failed")
		},
	}

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	// The job itself succeeded so Run returns nil; Update failure is logged but not returned
	require.NoError(t, err)
}

func TestRun_TopicPickerFails_FallsBackToGeneral(t *testing.T) {
	singleCombination(t)

	var capturedTopic string
	gen := &mockGenerator{fn: func(_ context.Context, params questions.GenerateParams) ([]domain.Question, error) {
		capturedTopic = params.Topic
		return []domain.Question{makeQuestion()}, nil
	}}
	picker := &mockPicker{fn: func(_ domain.Subject, _ int, _ domain.GameType) (string, error) {
		return "", errors.New("topic picker error")
	}}
	jobRepo := &mockJobRepo{}

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, &mockNotifier{}, picker, defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "general", capturedTopic)
}

func TestRun_ContextAlreadyCancelled(t *testing.T) {
	singleCombination(t)

	genCalled := false
	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		genCalled = true
		return nil, nil
	}}
	jobRepo := &mockJobRepo{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	_ = job.Run(ctx)

	assert.False(t, genCalled, "generator should not be called when context is already cancelled")
	assert.Equal(t, 0, jobRepo.savedRun.CombinationsDone)
}

func TestRun_ContextCancelledDuringDelay(t *testing.T) {
	twoCombinations(t)

	callCount := 0
	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		callCount++
		return []domain.Question{makeQuestion()}, nil
	}}

	cfg := Config{
		BatchSize:         1,
		MaxPerCombination: 10,
		CombinationDelay:  50 * time.Millisecond,
	}
	jobRepo := &mockJobRepo{}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first combination completes (during the delay for the second)
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, &mockNotifier{}, defaultPicker(), cfg, discardLogger())
	_ = job.Run(ctx)

	assert.Equal(t, 1, callCount, "only first combination should run before cancel")
}

func TestRun_NotifierAlertError_DoesNotMaskReturn(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, errors.New("gen error")
	}}
	notifier := &mockNotifier{alertErr: errors.New("alert failed")}
	jobRepo := &mockJobRepo{}

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, notifier, defaultPicker(), defaultConfig(), discardLogger())
	err := job.Run(context.Background())

	// job failure error should still be returned even though alert also failed
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
	assert.True(t, notifier.called)
}

func TestRun_QuestionsGeneratedCount(t *testing.T) {
	singleCombination(t)

	gen := &mockGenerator{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return []domain.Question{makeQuestion(), makeQuestion(), makeQuestion()}, nil
	}}
	jobRepo := &mockJobRepo{}

	job := NewQuestionGeneratorJob(gen, &mockQRepo{}, jobRepo, &mockNotifier{}, defaultPicker(), defaultConfig(), discardLogger())
	require.NoError(t, job.Run(context.Background()))

	assert.Equal(t, 3, jobRepo.savedRun.QuestionsGenerated)
}
