package app

// M6 · 设置与偏好。
//
// settingsPage itself lives in settings_views.go; this file owns the
// persisted preferences and the M6 blocks that have no block of their own:
// the account row, the first-run questionnaire and the help sections.

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
)

// prefsState is the persisted preference block plus the M6 pages App carries
// here because the shell owns only the feed and prefs blocks.
type prefsState struct {
	// loaded is true once app_meta has been read.
	loaded bool

	profile    profileState
	onboarding onboardingState
	help       helpState
}

func (s *prefsState) init() {
	s.onboarding.init()
	s.help.init()
}

// prefsData is app_meta["prefs"]: the appearance settings that survive a
// restart. AI keys never come near this blob.
type prefsData struct {
	Motion string `json:"motion"`
	Theme  string `json:"theme"`
	// ThemeName is the chosen Kero theme; Theme stays the light/dark mode.
	ThemeName string `json:"kero_theme"`
	// RecipeColumns is the table mode's column order and widths, so a
	// dragged column survives a restart.
	RecipeColumns *ui.TableLayout `json:"recipe_columns,omitempty"`
	// PantryOff hides the pantry/inventory surfaces; old blobs have no key,
	// which means the pantry is on.
	PantryOff bool `json:"pantry_off"`
}

// motionNames are the stored values of the motion setting; motionLabels are
// what the settings control shows.
var (
	motionNames  = []string{"full", "lite", "off"}
	motionLabels = []string{"完整", "减弱", "关闭"}

	themeNames  = []string{"auto", "light", "dark"}
	themeLabels = []string{"跟随系统", "浅色", "深色"}
)

// motionIndex maps a stored motion value to the segmented control's index.
func motionIndex(motion string) int {
	switch motion {
	case "lite":
		return 1
	case "off":
		return 2
	default:
		return 0
	}
}

// motionName maps the segmented control's index to the stored value.
func motionName(index int) string {
	if index < 0 || index >= len(motionNames) {
		return motionNames[0]
	}
	return motionNames[index]
}

// themeIndex maps a stored appearance to the segmented control's index.
func themeIndex(theme string) int {
	switch theme {
	case "light":
		return 1
	case "dark":
		return 2
	default:
		return 0
	}
}

// themeName maps the segmented control's index to the stored value.
func themeName(index int) string {
	if index < 0 || index >= len(themeNames) {
		return themeNames[0]
	}
	return themeNames[index]
}

// loadPrefsState reads the persisted preferences from app_meta and applies
// them before the first shell frame.
func (a *App) loadPrefsState(ctx context.Context, store repository.Repository) error {
	var data prefsData
	if _, err := store.MetaGetJSON(ctx, "prefs", &data); err != nil {
		return err
	}
	motion := data.Motion
	if motion != "lite" && motion != "off" {
		motion = "full"
	}
	theme := data.Theme
	if theme != "light" && theme != "dark" {
		theme = "auto"
	}
	// Old preference blobs have no kero_theme: they fall back to paper.
	name := data.ThemeName
	if !design.ValidTheme(name) {
		name = design.DefaultTheme
	}
	index := motionIndex(motion)
	tindex := themeIndex(theme)
	a.post(func() {
		a.motion = motion
		a.motionIndex = index
		a.theme = theme
		a.themeIndex = tindex
		a.themeName = name
		a.pantryOff = data.PantryOff
		if data.RecipeColumns != nil {
			a.recipeTable.Columns = *data.RecipeColumns
			a.recipeEdit.savedColumns = *data.RecipeColumns
		}
		if a.pantryOff {
			a.sichu.stockPolicy = sichuStockAllow
			// The shell has not drawn yet: open on the recipes page instead
			// of the hidden pantry.
			if a.pagePath == "" && strings.HasPrefix(a.router.Path(), "/pantry") {
				a.router.Replace("/recipes")
			}
		}
		a.prefs.loaded = true
		a.applyNativeTheme()
	})
	return nil
}

// applyNativeTheme tells MyGo the chosen appearance, so the window frame,
// title bar and menus follow the same mode as the content. Without a window
// (tests) the page theme alone is enough.
func (a *App) applyNativeTheme() {
	if a.win == nil {
		return
	}
	switch a.theme {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// setPantryOff closes or reopens every pantry door. The data stays in the
// database; only the UI and the settlement deduction are gated.
func (a *App) setPantryOff(off bool) {
	a.pantryOff = off
	a.recipeAvail = ""
	a.recipeSelected = -1
	if off {
		a.sichu.stockPolicy = sichuStockAllow
		if a.recipeSort.Column == "stock" {
			a.recipeSort = ui.SortOrder{}
		}
	}
	a.computeRecipeMatches()
	a.recomputeRecipeView()
	a.computeShopping()
	a.refreshSichuDashboard(time.Now())
	if off && strings.HasPrefix(a.router.Path(), "/pantry") {
		a.router.Replace("/recipes")
	}
	a.savePrefs()
	if off {
		a.notice = "仓库页已关闭；菜式与私厨照常使用"
	} else {
		a.notice = "仓库页已恢复"
	}
}

// savePrefs writes the appearance settings after a change.
func (a *App) savePrefs() {
	store := a.store
	if store == nil {
		return
	}
	cols := a.recipeTable.Columns
	data := prefsData{
		Motion: a.motion, Theme: a.theme, ThemeName: a.themeName,
		RecipeColumns: &cols, PantryOff: a.pantryOff,
	}
	go func() {
		if err := store.MetaSetJSON(context.Background(), "prefs", data); err != nil {
			a.post(func() { a.notice = "设置没有写入：" + err.Error() })
		}
	}()
}
