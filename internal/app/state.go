package app

import (
	"context"

	"github.com/gij072858-cmd/foodbox-go/internal/repository"
)

// initFeatureState prepares the per-feature state blocks. Each feature file
// owns its block's init, so milestones land without rewriting App itself.
func (a *App) initFeatureState() {
	a.recipeEdit.init()
	a.sched.init()
	a.sichu.init()
	a.ai.init()
	a.nutrition.init()
	a.feed.init()
	a.prefs.init()
}

// loadFeatureState reads every feature's persisted state after bootstrap or
// a restore. The loaders run on bootstrap's goroutine and publish view state
// through a.post; they return only hard errors.
func (a *App) loadFeatureState(ctx context.Context, store repository.Repository) error {
	for _, load := range []func(context.Context, repository.Repository) error{
		a.loadRecipeState,
		a.loadScheduleState,
		a.loadSichuState,
		a.loadNutritionState,
		a.loadFeedState,
		a.loadProfileState,
		a.loadPrefsState,
	} {
		if err := load(ctx, store); err != nil {
			return err
		}
	}
	return nil
}
