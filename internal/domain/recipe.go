package domain

import "strconv"

// Difficulty is a recipe's difficulty, as the seed data names it.
type Difficulty string

const (
	DifficultyEasy   Difficulty = "easy"
	DifficultyMedium Difficulty = "medium"
	DifficultyHard   Difficulty = "hard"
)

// Ingredient is one line of a recipe's ingredient list.
type Ingredient struct {
	Name             string   `json:"name"`
	Amount           float64  `json:"amount"`
	Unit             string   `json:"unit"`
	Required         bool     `json:"isRequired"`
	Seasoning        bool     `json:"isSeasoning"`
	Grams            *float64 `json:"grams"`
	AmountProvenance string   `json:"amountProvenance"`
}

// RecipeSource is where a seeded recipe came from.
type RecipeSource struct {
	Platform string `json:"platform"`
	Author   string `json:"author"`
	URL      string `json:"url"`
}

// Recipe is a dish. Steps are loaded with it for the detail page.
type Recipe struct {
	ID              int64         `json:"-"`
	Name            string        `json:"name"`
	Cuisine         string        `json:"cuisine"`
	Region          string        `json:"region"`
	MainCategory    string        `json:"mainCategory"`
	DurationMinutes *float64      `json:"durationMinutes"`
	Difficulty      Difficulty    `json:"difficulty"`
	BaseServings    int           `json:"baseServings"`
	SeasonMonths    []int         `json:"seasonMonths"`
	Ingredients     []Ingredient  `json:"ingredients"`
	Intro           string        `json:"intro"`
	Source          *RecipeSource `json:"source"`
	Steps           []Step        `json:"steps"`
	VersionGroup    string        `json:"versionGroup"`
	IsDish          bool          `json:"isDish"`
}

// Step is one instruction of a recipe.
type Step struct {
	ID              int64    `json:"-"`
	RecipeID        int64    `json:"-"`
	OrderIndex      int      `json:"orderIndex"`
	Instruction     string   `json:"instruction"`
	DurationMinutes *float64 `json:"durationMinutes"`
	Resources       []string `json:"resources"`
	SafetyTag       string   `json:"safetyTag"`
	PhotoPaths      []string `json:"photoPaths"`
	Note            string   `json:"note"`
	VideoPath       string   `json:"videoPath"`
	CompletionCue   string   `json:"completionCue"`
}

// DurationText returns "20 分钟" or "".
func (r Recipe) DurationText() string {
	if r.DurationMinutes == nil {
		return ""
	}
	return TrimFloat(*r.DurationMinutes) + " 分钟"
}

// TrimFloat formats a duration without a trailing ".0".
func TrimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		return s[:len(s)-2]
	}
	return s
}

// RecipeBrief is the label columns the feed and the annual review read
// without loading a dish's ingredients and steps.
type RecipeBrief struct {
	ID           int64
	Name         string
	Cuisine      string
	Region       string
	MainCategory string
}
