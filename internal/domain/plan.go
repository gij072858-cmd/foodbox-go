package domain

import "time"

// ScheduleStatus is a schedule row's state.
type ScheduleStatus string

const (
	SchedulePending ScheduleStatus = "pending"
	SchedulePlanned ScheduleStatus = "planned"
	ScheduleDone    ScheduleStatus = "done"
)

// ScheduleItem is one dish waiting to be cooked. Meal is the private
// kitchen's meal title (早餐/午餐/晚餐); an empty value is a dish added
// straight from the recipe page, which forms its own "本桌" group.
type ScheduleItem struct {
	ID         int64
	RecipeID   int64
	AddedAt    time.Time
	Servings   int
	Status     ScheduleStatus
	Meal       string
	PlannedFor *time.Time
}

// Record is one dish finished in a settlement. All dishes of one meal
// share CompletedAt and SessionSeconds.
type Record struct {
	ID             int64
	RecipeID       int64
	CompletedAt    time.Time
	Note           string
	XPGained       int
	SessionSeconds *int
}

// CookJournal is one day's meal diary.
type CookJournal struct {
	ID         int64
	Date       string
	PhotoPaths []string
	RecordIDs  []int64
	Note       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// UserProfile is the questionnaire profile (single row).
type UserProfile struct {
	StoveCount    int
	Cookware      string
	Hometown      string
	AgeRange      string
	Spicy         string
	Saltiness     string
	Restrictions  string
	AllergyNote   string
	HouseholdSize int
	HouseholdMix  string
	Onboarded     bool
	Version       int
}

// Profile is the account row (nickname, XP, lists).
type Profile struct {
	ID              int64
	Nickname        string
	AvatarPath      string
	BackgroundPath  string
	XP              int
	TotalMeals      int
	FavoriteRecipes []int64
	CookedRecipes   []int64
	WantedRecipes   []int64
	ReminderMode    string
}
