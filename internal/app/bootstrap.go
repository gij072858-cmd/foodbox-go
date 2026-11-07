package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/data/seed"
	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// bootstrap opens the database, migrates, imports the seed and loads what
// the views read. It runs on a goroutine; every state change is posted to
// the main thread.
func (a *App) bootstrap() {
	ctx := context.Background()

	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		a.fail(err)
		return
	}
	dbPath := filepath.Join(dir, "foodbox.db")
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		a.fail(err)
		return
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		a.fail(err)
		return
	}

	summary, err := seed.Import(ctx, store, func(p seed.Progress) {
		a.post(func() {
			a.stage = p.Stage
			a.progressDone = p.Done
			a.progressTotal = p.Total
		})
	})
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}

	recipes, err := store.ListAll(ctx)
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	steps, err := store.AllSteps(ctx)
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	items, err := store.ListItems(ctx)
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	catalog, err := loadCatalog()
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	packs, err := loadStarterPacks()
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	schedule, err := store.ListSchedule(ctx)
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	records, err := store.ListRecords(ctx, 200)
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	recordCount, err := store.RecordCount(ctx)
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	mealDays, err := store.MealDayCount(ctx)
	if err != nil {
		store.Close()
		a.fail(err)
		return
	}
	// Feature loaders read their own tables on this goroutine and publish
	// view state through a.post. They must use the store argument: a.store is
	// only set on the main thread, in the post below.
	if err := a.loadFeatureState(ctx, store); err != nil {
		store.Close()
		a.fail(err)
		return
	}

	byID := make(map[int64]domain.Recipe, len(recipes))
	for _, r := range recipes {
		byID[r.ID] = r
	}
	groups := groupCatalog(catalog)
	catalogIndex := make(map[string]domain.FoodItem, len(catalog))
	catalogNames := make([]string, 0, len(catalog))
	for _, it := range catalog {
		catalogIndex[it.Name] = it
		catalogNames = append(catalogNames, it.Name)
	}
	// The settings page's player and cookie lookups touch the disk and the
	// credential store; do them here, once, rather than under every frame.
	foundPlayer, haveCookie := a.probeVideo()
	a.post(func() {
		a.store = store
		a.recipes = recipes
		a.recipeByID = byID
		a.steps = steps
		a.catalog = catalog
		a.catalogGroups = groups
		a.catalogIndex = catalogIndex
		a.catalogNames = catalogNames
		a.packs = packs
		a.videoProbe = videoProbeState{
			found: foundPlayer, foundOK: true,
			cookie: haveCookie, cookieOK: true,
		}
		a.setItems(items)
		a.setSchedule(schedule)
		a.setRecords(records, recordCount, mealDays)
		a.stage = seed.StageDone
		a.progressDone = summary.Recipes
		a.progressTotal = summary.Recipes
		a.phase = PhaseReady
		a.recomputeRecipeView()
	})
}

func (a *App) fail(err error) {
	msg := err.Error()
	a.post(func() {
		a.bootErr = msg
		a.phase = PhaseError
	})
}

func (a *App) retry() {
	if a.store != nil {
		a.store.Close()
		a.store = nil
	}
	a.phase = PhaseLoading
	a.bootErr = ""
	a.stage = ""
	a.progressDone, a.progressTotal = 0, 0
	go a.bootstrap()
}

// groupCatalog groups the read-only catalog by category, keeping the first
// occurrence order of the JSON. It runs off the view.
func groupCatalog(items []domain.FoodItem) []domain.CatalogGroup {
	index := make(map[string]int)
	groups := make([]domain.CatalogGroup, 0, 12)
	for _, it := range items {
		i, ok := index[it.Category]
		if !ok {
			i = len(groups)
			index[it.Category] = i
			groups = append(groups, domain.CatalogGroup{Name: it.Category})
		}
		groups[i].Items = append(groups[i].Items, it)
	}
	return groups
}

func loadStarterPacks() ([]domain.StarterPack, error) {
	b, err := assets.Data("starter_packs.json")
	if err != nil {
		return nil, err
	}
	var file struct {
		Templates []domain.StarterPack `json:"templates"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return nil, fmt.Errorf("starter_packs.json: %w", err)
	}
	return file.Templates, nil
}

func loadCatalog() ([]domain.FoodItem, error) {
	b, err := assets.Data("food_items.json")
	if err != nil {
		return nil, err
	}
	var items []domain.FoodItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("food_items.json: %w", err)
	}
	if m, err := assets.LoadManifest(); err == nil {
		for i := range items {
			if entry, ok := m.Items[items[i].Name]; ok {
				items[i].Image = entry.Image
			}
		}
	}
	return items, nil
}
