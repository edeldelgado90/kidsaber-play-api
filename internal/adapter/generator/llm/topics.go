package llm

import (
	"fmt"
	"math/rand/v2"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// topicKey is the composite key for looking up applicable topics.
type topicKey struct {
	subject  domain.Subject
	grade    int
	gameType domain.GameType
}

// curriculum maps each (subject, grade, gameType) to a list of valid topic IDs.
// Source: 1.Analysis/v1/llm-topics-curriculum.md
// quick_calculation is excluded — it is always procedural.
var curriculum = map[topicKey][]string{
	// ── Matemáticas ───────────────────────────────────────────
	{domain.SubjectMathematics, 1, domain.GameTypeOptionMultiple}: {
		"numbers_up_to_99", "addition_within_20", "subtraction_within_20",
		"intro_multiplication_concept", "basic_shapes_2d", "compare_lengths_weights",
		"days_and_months", "telling_time_oclock",
	},
	{domain.SubjectMathematics, 1, domain.GameTypeFillInTheBlanks}: {
		"numbers_up_to_99", "addition_within_20", "subtraction_within_20",
		"intro_multiplication_concept", "days_and_months", "telling_time_oclock",
	},
	{domain.SubjectMathematics, 1, domain.GameTypeMatching}: {
		"basic_shapes_2d", "compare_lengths_weights", "days_and_months",
	},

	{domain.SubjectMathematics, 2, domain.GameTypeOptionMultiple}: {
		"numbers_up_to_999", "addition_with_carrying", "subtraction_with_borrowing",
		"multiplication_tables_2_3_4_5", "intro_division_sharing",
		"basic_fractions_half_third_quarter", "length_units_m_cm",
		"mass_units_kg_g", "capacity_units_l", "telling_time_half_quarter",
	},
	{domain.SubjectMathematics, 2, domain.GameTypeFillInTheBlanks}: {
		"numbers_up_to_999", "addition_with_carrying", "subtraction_with_borrowing",
		"multiplication_tables_2_3_4_5", "intro_division_sharing",
		"basic_fractions_half_third_quarter", "length_units_m_cm",
		"mass_units_kg_g", "capacity_units_l", "telling_time_half_quarter",
	},
	{domain.SubjectMathematics, 2, domain.GameTypeMatching}: {
		"basic_fractions_half_third_quarter", "length_units_m_cm", "mass_units_kg_g",
	},

	{domain.SubjectMathematics, 3, domain.GameTypeOptionMultiple}: {
		"numbers_up_to_9999", "addition_subtraction_large_numbers",
		"multiplication_tables_complete", "division_exact_inexact",
		"fractions_compare_add_simple", "intro_decimal_numbers_tenths",
		"perimeter_basic_shapes", "measurement_units_conversion",
	},
	{domain.SubjectMathematics, 3, domain.GameTypeFillInTheBlanks}: {
		"numbers_up_to_9999", "addition_subtraction_large_numbers",
		"multiplication_tables_complete", "division_exact_inexact",
		"fractions_compare_add_simple", "intro_decimal_numbers_tenths",
		"perimeter_basic_shapes", "measurement_units_conversion",
	},
	{domain.SubjectMathematics, 3, domain.GameTypeMatching}: {
		"fractions_compare_add_simple", "measurement_units_conversion",
	},

	{domain.SubjectMathematics, 4, domain.GameTypeOptionMultiple}: {
		"numbers_up_to_999999", "multi_digit_multiplication", "division_two_digit_divisor",
		"equivalent_fractions", "decimal_numbers_hundredths", "basic_proportionality",
		"area_flat_shapes", "coordinates_grid", "tables_and_bar_graphs",
	},
	{domain.SubjectMathematics, 4, domain.GameTypeFillInTheBlanks}: {
		"numbers_up_to_999999", "multi_digit_multiplication", "division_two_digit_divisor",
		"equivalent_fractions", "decimal_numbers_hundredths", "basic_proportionality",
		"area_flat_shapes", "coordinates_grid", "tables_and_bar_graphs",
	},
	{domain.SubjectMathematics, 4, domain.GameTypeMatching}: {
		"equivalent_fractions",
	},

	{domain.SubjectMathematics, 5, domain.GameTypeOptionMultiple}: {
		"numbers_up_to_millions", "powers_and_square_roots", "fractions_multiply_divide",
		"decimal_operations_all", "basic_percentages", "rule_of_three",
		"area_perimeter_polygons", "volume_basic_solids", "mean_and_mode",
	},
	{domain.SubjectMathematics, 5, domain.GameTypeFillInTheBlanks}: {
		"numbers_up_to_millions", "powers_and_square_roots", "fractions_multiply_divide",
		"decimal_operations_all", "basic_percentages", "rule_of_three",
		"area_perimeter_polygons", "volume_basic_solids", "mean_and_mode",
	},
	{domain.SubjectMathematics, 5, domain.GameTypeMatching}: {},

	{domain.SubjectMathematics, 6, domain.GameTypeOptionMultiple}: {
		"integer_numbers_negative", "combined_operations_fractions", "applied_percentages",
		"intro_equations", "scales_and_maps", "geometric_solids_volume",
		"basic_probability", "statistics_charts",
	},
	{domain.SubjectMathematics, 6, domain.GameTypeFillInTheBlanks}: {
		"integer_numbers_negative", "combined_operations_fractions", "applied_percentages",
		"intro_equations", "scales_and_maps", "geometric_solids_volume",
		"basic_probability", "statistics_charts",
	},
	{domain.SubjectMathematics, 6, domain.GameTypeMatching}: {
		"integer_numbers_negative", "geometric_solids_volume",
	},

	// ── Lengua Castellana y Literatura ───────────────────────
	{domain.SubjectLanguage, 1, domain.GameTypeOptionMultiple}: {
		"reading_syllables_words", "uppercase_lowercase", "sentence_final_punctuation",
		"nouns_gender_number", "definite_indefinite_articles", "basic_reading_comprehension",
	},
	{domain.SubjectLanguage, 1, domain.GameTypeFillInTheBlanks}: {
		"reading_syllables_words", "uppercase_lowercase", "sentence_final_punctuation",
		"nouns_gender_number", "definite_indefinite_articles", "basic_reading_comprehension",
	},
	{domain.SubjectLanguage, 1, domain.GameTypeMatching}: {
		"uppercase_lowercase", "nouns_gender_number",
	},

	{domain.SubjectLanguage, 2, domain.GameTypeOptionMultiple}: {
		"sentence_types", "common_proper_nouns", "adjectives_basic",
		"verb_tenses_intro", "synonyms_antonyms", "syllable_separation",
	},
	{domain.SubjectLanguage, 2, domain.GameTypeFillInTheBlanks}: {
		"sentence_types", "common_proper_nouns", "adjectives_basic",
		"verb_tenses_intro", "synonyms_antonyms", "syllable_separation",
	},
	{domain.SubjectLanguage, 2, domain.GameTypeMatching}: {
		"sentence_types", "common_proper_nouns", "verb_tenses_intro", "synonyms_antonyms",
	},

	{domain.SubjectLanguage, 3, domain.GameTypeOptionMultiple}: {
		"narrative_descriptive_text", "subject_and_predicate",
		"verb_conjugation_present_perfect_future", "adjective_degrees",
		"punctuation_marks", "prefixes_and_suffixes", "spelling_b_v_h_cz",
	},
	{domain.SubjectLanguage, 3, domain.GameTypeFillInTheBlanks}: {
		"narrative_descriptive_text", "subject_and_predicate",
		"verb_conjugation_present_perfect_future", "adjective_degrees",
		"punctuation_marks", "prefixes_and_suffixes", "spelling_b_v_h_cz",
	},
	{domain.SubjectLanguage, 3, domain.GameTypeMatching}: {
		"subject_and_predicate", "verb_conjugation_present_perfect_future",
		"punctuation_marks", "prefixes_and_suffixes",
	},

	{domain.SubjectLanguage, 4, domain.GameTypeOptionMultiple}: {
		"text_types", "morphological_analysis_basic", "indicative_verb_conjugation",
		"simple_compound_sentences", "spelling_ge_gi_je_ji_ll_y",
		"derived_compound_words", "literary_genres_basic",
	},
	{domain.SubjectLanguage, 4, domain.GameTypeFillInTheBlanks}: {
		"text_types", "morphological_analysis_basic", "indicative_verb_conjugation",
		"simple_compound_sentences", "spelling_ge_gi_je_ji_ll_y",
		"derived_compound_words", "literary_genres_basic",
	},
	{domain.SubjectLanguage, 4, domain.GameTypeMatching}: {
		"text_types", "morphological_analysis_basic", "derived_compound_words", "literary_genres_basic",
	},

	{domain.SubjectLanguage, 5, domain.GameTypeOptionMultiple}: {
		"argumentative_text", "syntactic_analysis_basic", "subjunctive_and_imperative",
		"text_connectors", "accentuation_rules", "literary_figures_metaphor_simile",
		"literary_genres_theater_novel",
	},
	{domain.SubjectLanguage, 5, domain.GameTypeFillInTheBlanks}: {
		"argumentative_text", "syntactic_analysis_basic", "subjunctive_and_imperative",
		"text_connectors", "accentuation_rules", "literary_figures_metaphor_simile",
		"literary_genres_theater_novel",
	},
	{domain.SubjectLanguage, 5, domain.GameTypeMatching}: {
		"syntactic_analysis_basic", "text_connectors",
		"literary_figures_metaphor_simile", "literary_genres_theater_novel",
	},

	{domain.SubjectLanguage, 6, domain.GameTypeOptionMultiple}: {
		"functional_texts", "subordinate_clauses", "irregular_verbs",
		"homophones", "spanish_literature_periods", "stylistic_devices", "study_techniques",
	},
	{domain.SubjectLanguage, 6, domain.GameTypeFillInTheBlanks}: {
		"functional_texts", "subordinate_clauses", "irregular_verbs",
		"homophones", "spanish_literature_periods", "stylistic_devices", "study_techniques",
	},
	{domain.SubjectLanguage, 6, domain.GameTypeMatching}: {
		"subordinate_clauses", "irregular_verbs", "homophones",
		"spanish_literature_periods", "stylistic_devices",
	},

	// ── Inglés ────────────────────────────────────────────────
	{domain.SubjectEnglish, 1, domain.GameTypeOptionMultiple}: {
		"greetings_farewells", "numbers_1_to_10", "basic_colors",
		"domestic_animals_en", "classroom_vocabulary", "verb_to_be_singular", "personal_introduction",
	},
	{domain.SubjectEnglish, 1, domain.GameTypeFillInTheBlanks}: {
		"greetings_farewells", "numbers_1_to_10", "basic_colors",
		"domestic_animals_en", "classroom_vocabulary", "verb_to_be_singular", "personal_introduction",
	},
	{domain.SubjectEnglish, 1, domain.GameTypeMatching}: {
		"greetings_farewells", "numbers_1_to_10", "basic_colors",
		"domestic_animals_en", "classroom_vocabulary",
	},

	{domain.SubjectEnglish, 2, domain.GameTypeOptionMultiple}: {
		"numbers_1_to_20", "family_members_en", "body_parts_en",
		"clothing_basic_en", "verb_to_be_complete", "have_has_possession", "basic_questions_what_how_many",
	},
	{domain.SubjectEnglish, 2, domain.GameTypeFillInTheBlanks}: {
		"numbers_1_to_20", "family_members_en", "body_parts_en",
		"clothing_basic_en", "verb_to_be_complete", "have_has_possession", "basic_questions_what_how_many",
	},
	{domain.SubjectEnglish, 2, domain.GameTypeMatching}: {
		"numbers_1_to_20", "family_members_en", "body_parts_en", "clothing_basic_en",
	},

	{domain.SubjectEnglish, 3, domain.GameTypeOptionMultiple}: {
		"numbers_1_to_100", "house_and_rooms", "food_and_meals_en",
		"animals_and_habitats_en", "present_simple", "there_is_there_are", "descriptive_adjectives_en",
	},
	{domain.SubjectEnglish, 3, domain.GameTypeFillInTheBlanks}: {
		"numbers_1_to_100", "house_and_rooms", "food_and_meals_en",
		"animals_and_habitats_en", "present_simple", "there_is_there_are", "descriptive_adjectives_en",
	},
	{domain.SubjectEnglish, 3, domain.GameTypeMatching}: {
		"numbers_1_to_100", "house_and_rooms", "food_and_meals_en",
		"animals_and_habitats_en", "descriptive_adjectives_en",
	},

	{domain.SubjectEnglish, 4, domain.GameTypeOptionMultiple}: {
		"weather_en", "sports_and_activities_en", "jobs_and_professions_en",
		"present_continuous", "can_cant_ability", "past_simple_regular_verbs", "prepositions_of_place_en",
	},
	{domain.SubjectEnglish, 4, domain.GameTypeFillInTheBlanks}: {
		"weather_en", "sports_and_activities_en", "jobs_and_professions_en",
		"present_continuous", "can_cant_ability", "past_simple_regular_verbs", "prepositions_of_place_en",
	},
	{domain.SubjectEnglish, 4, domain.GameTypeMatching}: {
		"weather_en", "sports_and_activities_en", "jobs_and_professions_en",
		"past_simple_regular_verbs", "prepositions_of_place_en",
	},

	{domain.SubjectEnglish, 5, domain.GameTypeOptionMultiple}: {
		"past_simple_irregular_verbs", "going_to_future", "comparatives_and_superlatives",
		"environment_and_nature_en", "health_and_human_body_en", "adverbs_of_frequency",
	},
	{domain.SubjectEnglish, 5, domain.GameTypeFillInTheBlanks}: {
		"past_simple_irregular_verbs", "going_to_future", "comparatives_and_superlatives",
		"environment_and_nature_en", "health_and_human_body_en", "adverbs_of_frequency",
	},
	{domain.SubjectEnglish, 5, domain.GameTypeMatching}: {
		"past_simple_irregular_verbs", "environment_and_nature_en", "health_and_human_body_en",
	},

	{domain.SubjectEnglish, 6, domain.GameTypeOptionMultiple}: {
		"future_simple_will", "present_perfect_intro", "first_conditional",
		"cities_and_travel_en", "technology_and_daily_life_en", "writing_emails_descriptions_en",
		"reading_comprehension_longer_texts",
	},
	{domain.SubjectEnglish, 6, domain.GameTypeFillInTheBlanks}: {
		"future_simple_will", "present_perfect_intro", "first_conditional",
		"cities_and_travel_en", "technology_and_daily_life_en", "writing_emails_descriptions_en",
	},
	{domain.SubjectEnglish, 6, domain.GameTypeMatching}: {
		"cities_and_travel_en", "technology_and_daily_life_en",
	},

	// ── Conocimiento del Medio ────────────────────────────────
	{domain.SubjectScience, 1, domain.GameTypeOptionMultiple}: {
		"human_body_parts", "the_five_senses", "family_and_home",
		"domestic_and_wild_animals", "parts_of_a_plant", "weather_phenomena", "healthy_habits",
	},
	{domain.SubjectScience, 1, domain.GameTypeFillInTheBlanks}: {
		"human_body_parts", "the_five_senses", "family_and_home",
		"domestic_and_wild_animals", "parts_of_a_plant", "weather_phenomena", "healthy_habits",
	},
	{domain.SubjectScience, 1, domain.GameTypeMatching}: {
		"human_body_parts", "the_five_senses", "family_and_home",
		"domestic_and_wild_animals", "parts_of_a_plant", "weather_phenomena",
	},

	{domain.SubjectScience, 2, domain.GameTypeOptionMultiple}: {
		"living_nonliving_things", "animal_classification", "water_cycle",
		"landscapes", "materials_and_properties", "the_local_community", "past_and_present",
	},
	{domain.SubjectScience, 2, domain.GameTypeFillInTheBlanks}: {
		"living_nonliving_things", "animal_classification", "water_cycle",
		"landscapes", "materials_and_properties", "the_local_community", "past_and_present",
	},
	{domain.SubjectScience, 2, domain.GameTypeMatching}: {
		"living_nonliving_things", "animal_classification", "water_cycle",
		"landscapes", "materials_and_properties", "past_and_present",
	},

	{domain.SubjectScience, 3, domain.GameTypeOptionMultiple}: {
		"solar_system_planets", "states_of_matter", "rocks_and_minerals",
		"ecosystems", "intro_to_cell", "local_and_regional_history", "local_institutions",
	},
	{domain.SubjectScience, 3, domain.GameTypeFillInTheBlanks}: {
		"solar_system_planets", "states_of_matter", "rocks_and_minerals",
		"ecosystems", "intro_to_cell", "local_and_regional_history", "local_institutions",
	},
	{domain.SubjectScience, 3, domain.GameTypeMatching}: {
		"solar_system_planets", "states_of_matter", "rocks_and_minerals",
		"ecosystems", "intro_to_cell", "local_institutions",
	},

	{domain.SubjectScience, 4, domain.GameTypeOptionMultiple}: {
		"digestive_respiratory_circulatory_systems", "nutrition_in_living_beings",
		"forces_and_motion", "basic_electricity", "atmosphere_and_climate",
		"spain_physical_geography", "history_ancient_medieval_spain",
	},
	{domain.SubjectScience, 4, domain.GameTypeFillInTheBlanks}: {
		"digestive_respiratory_circulatory_systems", "nutrition_in_living_beings",
		"forces_and_motion", "basic_electricity", "atmosphere_and_climate",
		"spain_physical_geography", "history_ancient_medieval_spain",
	},
	{domain.SubjectScience, 4, domain.GameTypeMatching}: {
		"digestive_respiratory_circulatory_systems", "nutrition_in_living_beings",
		"forces_and_motion", "basic_electricity", "atmosphere_and_climate",
		"spain_physical_geography", "history_ancient_medieval_spain",
	},

	{domain.SubjectScience, 5, domain.GameTypeOptionMultiple}: {
		"nervous_reproductive_excretory_systems", "plant_photosynthesis_reproduction",
		"energy_sources", "climate_change_biodiversity", "spain_political_geography",
		"history_modern_contemporary_spain", "spanish_constitution",
	},
	{domain.SubjectScience, 5, domain.GameTypeFillInTheBlanks}: {
		"nervous_reproductive_excretory_systems", "plant_photosynthesis_reproduction",
		"energy_sources", "climate_change_biodiversity", "spain_political_geography",
		"history_modern_contemporary_spain", "spanish_constitution",
	},
	{domain.SubjectScience, 5, domain.GameTypeMatching}: {
		"nervous_reproductive_excretory_systems", "plant_photosynthesis_reproduction",
		"energy_sources", "spain_political_geography", "history_modern_contemporary_spain",
	},

	{domain.SubjectScience, 6, domain.GameTypeOptionMultiple}: {
		"health_disease_vaccines", "puberty_changes", "mixtures_and_solutions",
		"light_and_sound", "internet_and_society", "europe_geography_eu",
		"world_history_20th_21st_century", "basic_financial_education",
	},
	{domain.SubjectScience, 6, domain.GameTypeFillInTheBlanks}: {
		"health_disease_vaccines", "puberty_changes", "mixtures_and_solutions",
		"light_and_sound", "internet_and_society", "europe_geography_eu",
		"world_history_20th_21st_century", "basic_financial_education",
	},
	{domain.SubjectScience, 6, domain.GameTypeMatching}: {
		"health_disease_vaccines", "mixtures_and_solutions", "light_and_sound",
		"europe_geography_eu", "world_history_20th_21st_century",
	},
}

// TopicPicker selects a random valid topic for a given subject/grade/gameType combination.
type TopicPicker struct{}

// NewTopicPicker returns a TopicPicker ready for use.
func NewTopicPicker() *TopicPicker {
	return &TopicPicker{}
}

// Pick returns a random topic ID for the given combination.
// Returns an error if no topics are available (should not happen for valid inputs).
func (tp *TopicPicker) Pick(subject domain.Subject, grade int, gameType domain.GameType) (string, error) {
	key := topicKey{subject: subject, grade: grade, gameType: gameType}
	topics, ok := curriculum[key]
	if !ok || len(topics) == 0 {
		return "", fmt.Errorf("no topics available for %s grade %d %s", subject, grade, gameType)
	}
	return topics[rand.IntN(len(topics))], nil
}

// Topics returns all topic IDs for a given combination (used in tests).
func (tp *TopicPicker) Topics(subject domain.Subject, grade int, gameType domain.GameType) []string {
	key := topicKey{subject: subject, grade: grade, gameType: gameType}
	return curriculum[key]
}
