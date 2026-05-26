package llm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/llm"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

func TestTopicPicker_ReturnsTopicForAllCombinations(t *testing.T) {
	picker := llm.NewTopicPicker()

	gameTypes := []domain.GameType{
		domain.GameTypeOptionMultiple,
		domain.GameTypeFillInTheBlanks,
		domain.GameTypeMatching,
	}
	subjects := []domain.Subject{
		domain.SubjectMathematics,
		domain.SubjectLanguage,
		domain.SubjectEnglish,
		domain.SubjectScience,
	}

	for _, s := range subjects {
		for grade := 1; grade <= 6; grade++ {
			for _, gt := range gameTypes {
				topics := picker.Topics(s, grade, gt)
				// Some matching combinations have zero topics (e.g. mathematics/5/matching)
				// We only require Pick to succeed when topics are available.
				if len(topics) > 0 {
					topic, err := picker.Pick(s, grade, gt)
					require.NoError(t, err,
						"Pick should succeed for %s grade %d %s", s, grade, gt)
					assert.NotEmpty(t, topic,
						"topic should not be empty for %s grade %d %s", s, grade, gt)
					assert.Contains(t, topics, topic,
						"returned topic should be in the topics list")
				}
			}
		}
	}
}

func TestTopicPicker_ErrorWhenNoTopicsAvailable(t *testing.T) {
	picker := llm.NewTopicPicker()

	// mathematics grade 5 matching has no topics in the curriculum
	_, err := picker.Pick(domain.SubjectMathematics, 5, domain.GameTypeMatching)
	assert.Error(t, err)
}

func TestTopicPicker_PickIsDeterministicInSet(t *testing.T) {
	picker := llm.NewTopicPicker()

	topics := picker.Topics(domain.SubjectLanguage, 3, domain.GameTypeOptionMultiple)
	require.NotEmpty(t, topics)

	// Run 100 times — all results should be in the known topics list
	for i := 0; i < 100; i++ {
		topic, err := picker.Pick(domain.SubjectLanguage, 3, domain.GameTypeOptionMultiple)
		require.NoError(t, err)
		assert.Contains(t, topics, topic)
	}
}
