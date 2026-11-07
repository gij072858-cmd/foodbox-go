package domain

// RunSession is one cooking run's progress. It is persisted so closing the
// app mid-run does not lose the place: the run page reopens on the same step
// with the same elapsed time.
type RunSession struct {
	RecipeIDs []int64 `json:"recipeIds"`
	// Meal is the meal group the run belongs to (早餐/午餐/晚餐); empty means
	// a table assembled directly from the recipe page.
	Meal string `json:"meal,omitempty"`
	// Step is the index into the flattened lyric lines.
	Step int `json:"step"`
	// StartedAt is when the run began, Unix seconds.
	StartedAt int64 `json:"startedAt"`
	Paused    bool  `json:"paused"`
	// PausedFor is how many seconds were spent paused.
	PausedFor int   `json:"pausedFor"`
	UpdatedAt int64 `json:"updatedAt"`
	// Hang is the tray of countdowns hung above the flow, in the order they
	// were hung. The original FoodBox saved the timers but not that list, so
	// a restart came back with a running clock and an empty tray
	// (app/lib/features/schedule/run_session_store.dart). It is part of the
	// JSON blob, not a table, so no schema version moves.
	Hang []HangSnapshot `json:"hang,omitempty"`
}

// HangSnapshot is one hung countdown as the run saved it. A running timer is
// saved as the moment it ends, so closing the app does not buy the cook any
// time; a frozen one is saved as the seconds it still owes.
type HangSnapshot struct {
	Line      int    `json:"line"`
	Title     string `json:"title"`
	EndAt     int64  `json:"endAt"`
	Total     int    `json:"totalSeconds"`
	Paused    bool   `json:"paused"`
	Remaining int    `json:"pausedRemainingSeconds"`
}

// ElapsedSeconds is the run's wall-clock time, minus paused time.
func (s RunSession) ElapsedSeconds(now int64) int {
	if s.StartedAt == 0 {
		return 0
	}
	elapsed := int(now-s.StartedAt) - s.PausedFor
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

// Empty reports whether there is no run in progress.
func (s RunSession) Empty() bool { return len(s.RecipeIDs) == 0 }
