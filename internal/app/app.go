// Package app holds FoodBox's state and its views. Everything here runs on
// the main thread: the views, and asynchronous work posted through the
// window's Update.
package app

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/data/seed"
	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
)

// Phase is where startup is.
type Phase int

const (
	PhaseLoading Phase = iota
	PhaseReady
	PhaseError
)

// App is the state the window shows.
type App struct {
	win   *mygo.Window
	store repository.Repository

	router *ui.Router

	// page is the top-level section the sidebar selects.
	page       string
	query      string
	motion     string
	theme      string
	themeName  string // the Kero theme (petal…plain), independent of the light/dark mode
	themeIndex int
	// pantryOff hides every pantry/inventory surface and makes settlement
	// leave stock alone. The data is kept, only the doors are closed.
	pantryOff bool
	level     design.Level

	// pageAt is when the current route opened; pagePath tells the next
	// frame whether it changed. List rows use it to stagger in once.
	pageAt   time.Time
	pagePath string
	// pageDir is the way the route last changed: 1 deeper, -1 back up, 0
	// across. pageEnterAt is when that change happened; the page enters
	// over design.Page from there, once, and the router switches at once.
	pageDir     int
	pageEnterAt time.Time
	// crumbChosen is the breadcrumb the pointer chose this frame.
	crumbChosen int
	// winW and winH are the window size this frame, read at the root
	// context: narrow layouts use them instead of guessing from a row.
	winW, winH float32
	// navTypeahead remembers what was typed at the sidebar and when, so
	// one letter picks a destination and the same letter walks the rest.
	navTypeaheadText string
	navTypeaheadAt   time.Time
	navTypeaheadStep int

	// search is the sidebar field, so Cmd/Ctrl+F can put the caret in it.
	search ui.Handle

	// videoOpen shows a recipe's source page in an in-app web view; the
	// source links are web pages, not media files.
	videoOpen bool
	videoURL  string
	videoView *mygo.WebView
	// videoBusy is a resolve in flight; videoPending is the page to show in
	// the web view if the resolve could not be played.
	videoBusy    bool
	videoPending string
	// videoCmd is an external player this app started, so it can be stopped
	// and noticed when it exits.
	videoCmd    *exec.Cmd
	videoCmdURL string
	// videoCookie is the settings field's buffer; only its saved form, in
	// the credential store, is ever read back.
	videoCookie string
	// videoProbe caches what the settings page used to look up every frame:
	// finding a player scans the PATH and the disk, and reading the cookie
	// opens the credential store. Both run once, off the main thread.
	videoProbe videoProbeState

	// scrolls keeps one scroll position per route, so a page comes back
	// where it was and its edge fades know what is under them.
	scrolls map[string]*ui.ScrollState

	// buildingPath is the route whose page is being built. During a page
	// transition the router builds two pages in one frame, and each must
	// own a different scroll state.
	buildingPath string

	// motionIndex is the settings segmented control's choice.
	motionIndex int

	// sheetOpen shows the M0 sheet probe.
	sheetOpen bool

	// lyric is the scroll of the schedule page's lyric flow; lyricFocus is
	// the step the prev/next controls keep centered.
	lyric      ui.ScrollState
	lyricFocus int

	// Startup and data.
	phase         Phase
	stage         seed.Stage
	progressDone  int
	progressTotal int
	bootErr       string
	recipes       []domain.Recipe
	recipeByID    map[int64]domain.Recipe
	steps         map[int64][]domain.Step
	catalog       []domain.FoodItem
	catalogGroups []domain.CatalogGroup
	itemCount     int
	recipesList   ui.ListState
	catalogList   ui.ListState
	catalogGrid   ui.GridState
	catalogGroup  int

	// Pantry (M2): items are loaded as a snapshot; the filtered view is
	// rebuilt after every write, never while drawing.
	items         []domain.Item
	itemsByID     map[int64]domain.Item
	pantryView    []domain.Item
	pantryQuery   string
	pantryFilter  string
	pantryGrid    ui.GridState
	reconcileList ui.ListState
	itemDetail    int64
	detailOpen    bool
	reconciling   bool
	notice        string
	noticeSeen    string

	// Catalog and starter packs.
	catalogView      []domain.FoodItem
	catalogQuery     string
	catalogFindOpen  bool
	catalogFindQuery string
	catalogFindIndex int
	catalogIndex     map[string]domain.FoodItem
	catalogNames     []string
	packs            []domain.StarterPack
	packID           string
	packOptional     bool
	packPicks        map[string]bool
	packSplit        float32
	packList         ui.ListState

	// Inline quick edit.
	editID       int64
	editMode     int
	editQty      float64
	editQtyText  string
	editUnit     string
	editGranular int
	editShelf    int
	editOpen     bool

	// Custom item editor.
	customOpen  bool
	customName  string
	customCat   string
	customUnit  string
	customQty   string
	customShelf string

	shopping []domain.ShoppingLine
	shopList ui.ListState

	// recipeView is what the recipe page lists: recipes filtered by the
	// sidebar search and the selected facet. It is rebuilt off the view
	// when either changes, never while building a frame.
	recipeView     []domain.Recipe
	recipeFilter   string
	recipeAvail    string
	recipeTable    ui.ListState
	recipeSelected int
	recipeSort     ui.SortOrder
	ingredientList ui.ListState

	// matcher and recipeMatches are derived once per pantry change.
	matcher       *cookmatch.Matcher
	recipeMatches map[int64]cookmatch.Result
	canCook       int
	almost        int
	farAway       int
	shopExtras    []domain.ShoppingLine

	// dishNutCache holds per-dish nutrition estimates, filled lazily as the
	// recipe list and hero ask for them, cleared with the dish snapshot.
	dishNutCache map[int64]dishNutrition

	// Schedule and records (M4).
	schedule         []domain.ScheduleItem
	scheduleSelected int64
	records          []domain.Record
	recordCount      int
	mealDays         int
	sessionStarted   time.Time
	favorites        map[int64]bool

	// Feature state blocks. Each milestone owns one file that defines its
	// block and its init; App only carries the value.
	recipeEdit recipeEditState
	sched      scheduleState
	sichu      sichuState
	ai         aiState
	nutrition  nutritionState
	feed       feedState
	prefs      prefsState
	legacy     legacyImportState

	// images is the single LRU for category and pantry images. Pages ask it
	// for decoded bitmaps; decoding runs off the main thread.
	images *assets.Images
}

// New makes the app with the pantry open.
func New() *App {
	// The framework's own controls (a calendar's months, a menu's Copy)
	// follow this language; imported here rather than in main so headless
	// testers and the screenshots speak it too.
	mygo.App.SetLanguages("zh-Hans")
	router := ui.NewRouter("/pantry")
	a := &App{
		router:        router,
		motion:        "full",
		theme:         "auto",
		themeName:     design.DefaultTheme,
		phase:         PhaseLoading,
		recipeByID:    map[int64]domain.Recipe{},
		steps:         map[int64][]domain.Step{},
		favorites:     map[int64]bool{},
		images:        assets.NewImages(160),
		itemsByID:     map[int64]domain.Item{},
		catalogIndex:  map[string]domain.FoodItem{},
		matcher:       cookmatch.MustNew(),
		recipeMatches: map[int64]cookmatch.Result{},
		dishNutCache:  map[int64]dishNutrition{},
		scrolls:       map[string]*ui.ScrollState{},
		packPicks:     map[string]bool{},
		packSplit:     220,
	}
	a.initFeatureState()
	a.recipeTable.Sort = &a.recipeSort
	a.recipeTable.Key = func(row int) any {
		if row >= 0 && row < len(a.recipeView) {
			return a.recipeView[row].ID
		}
		return row
	}
	a.recipeSelected = -1
	a.recipeTable.Selected = &a.recipeSelected
	a.recipeTable.CellMenu = func(m *ui.Menu, row, col int) {
		if row < 0 || row >= len(a.recipeView) {
			return
		}
		rec := a.recipeView[row]
		if m.Item("打开详情").Chosen() {
			a.router.Push(fmt.Sprintf("/recipes/%d", rec.ID))
		}
		if m.Item("加入掌序").Chosen() {
			a.addToSchedule(rec)
		}
		m.Separator()
		if m.Item("收藏").Checked(a.favorites[rec.ID]).Chosen() {
			a.toggleFavorite(rec)
		}
	}
	return a
}

// Run opens the window and runs the app. A second launch forwards its
// arguments to the first window instead of opening a second writer on the
// same database.
func Run() error {
	// MyGo's own controls, menus and text shaping follow this language; the
	// locale catalog imported in main.go only registers its translations.
	mygo.App.SetLanguages("zh-Hans")
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	a := New()
	mygo.App.OnSecondInstance(func(_ []string, _ string) {
		if a.win != nil {
			a.win.Focus()
		}
	})
	mygo.App.OnQuit(func() {
		if a.store != nil {
			a.store.Close()
		}
	})
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "食匣",
			Width:     1280,
			Height:    860,
			MinWidth:  1024,
			MinHeight: 700,
			// The app is opened to be used: the first launch starts maximized
			// and in front; later launches restore the window state.
			Maximized: true,
			StateKey:  "main",
			// Cloudy's chrome (app.go:282-298): the system's title bar is gone
			// and the page draws that strip itself, while MyGo keeps drawing
			// the caption buttons over it — so drag, double-click zoom, the
			// resize edges and Windows' snap layouts all stay the system's.
			// ui.Context.TitleBar reports the room the buttons take; the shell
			// keeps clear of it.
			TitleBarStyle:  mygo.TitleBarHidden,
			TitleBarHeight: design.TitleBand,
			Content:        ui.View(a.View),
		})
		a.win.Focus()
		go a.bootstrap()
	})
	return mygo.App.Run()
}

// View builds the window for this frame.
func (a *App) View(c *ui.Context) {
	a.winW, a.winH = c.Size()
	t := design.ApplyTheme(c, a.theme, a.themeName)
	p := design.PaletteFor(a.themeName, t.Dark)
	// Settle before anything is built: "off" must reach every pose and
	// transition in this frame, not just the pages that check the level.
	a.level = design.LevelFor(c, a.motion)
	a.level.Apply(c)
	a.shortcuts(c)
	// A notice is feedback, not furniture. The framework's toast owns the
	// timing, stacking and fade; watching the string here means every
	// writer gets one without each page rendering it.
	if a.notice != a.noticeSeen {
		a.noticeSeen = a.notice
		if a.notice != "" {
			c.Toast(a.notice)
		}
	}
	if a.phase != PhaseReady {
		a.startupView(c, p)
		return
	}
	if a.onboardingRequired() {
		a.onboardingView(c, p)
		return
	}
	a.syncPage()
	if path := a.router.Path(); path != a.pagePath {
		old := a.pagePath
		a.pagePath = path
		switch {
		case old == "":
			// The first page is simply there; it has nowhere to come from.
			a.pageDir, a.pageEnterAt = 0, time.Time{}
			a.pageAt = c.Now()
		default:
			a.pageDir = routeDirection(routeDepth(old), routeDepth(path))
			a.pageEnterAt = c.Now()
			if a.pageDir < 0 {
				// Back to a list already built: its rows are there already.
				a.pageAt = time.Time{}
			} else {
				a.pageAt = c.Now()
			}
		}
	}
	// The router swaps at once; the page itself carries the motion. The
	// stock TransitionAuto slides a whole page across and rebuilds both
	// pages every frame below a section root, which is the stutter; Cloudy
	// moves only the arriving page, a little (design.PageEnter).
	a.router.Transition = ui.TransitionNone

	root := ui.Row(c).Fill().AlignItems(ui.Stretch)
	design.WashBackdrop(c, root, p)
	// The run screen is immersive: with the fire lit, the cook is at the
	// stove, not browsing. The original FoodBox does this by pushing
	// schedule/run on the root navigator, above the shell, and the shell
	// notices it is covered (app/lib/core/app_router.dart:296-301 and
	// app/lib/features/shell/app_shell.dart:338-350). Same effect here:
	// the page is built without the sidebar or the top bar.
	immersive := a.sched.runActive && a.pagePath == "/schedule/run"
	if immersive {
		column := ui.Column(c).Fill().MinWidth(0)
		column.Children(func() {
			a.router.View(c, func(r *ui.Route) {
				a.route(c, r, p)
				design.PageEnter(c, a.level, a.pageEnterAt, a.pageDir, r.Page())
			})
		})
		// Over the window, the same place the flying cover is drawn from:
		// a page's own box is only as tall as its content, and the band has
		// to cover a share of the screen, not of the page.
		a.hangDropZone(c, p)
		a.sheet(c, p)
		a.videoOverlay(c, p)
		return
	}
	root.Children(func() {
		a.sidebar(c, p)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			a.topBar(c)
			a.router.View(c, func(r *ui.Route) {
				a.route(c, r, p)
				design.PageEnter(c, a.level, a.pageEnterAt, a.pageDir, r.Page())
			})
		})
	})
	a.sheet(c, p)
	a.videoOverlay(c, p)
}

// navShortcuts are the sections Cmd/Ctrl+1…7 reach, in sidebar order.
var (
	navShortcuts = []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7}
	navPaths     = []string{"/pantry", "/recipes", "/sichu", "/schedule", "/feed", "/nutrition", "/profile"}
)

// homePath is where the app lands when nothing else says otherwise.
func (a *App) homePath() string {
	if a.pantryOff {
		return "/recipes"
	}
	return "/pantry"
}

// navTargets is the section table the number shortcuts walk: with the pantry
// closed, Ctrl+1 opens 菜式 and every following number shifts with it.
func (a *App) navTargets() []string {
	if !a.pantryOff {
		return navPaths
	}
	return navPaths[1:]
}

// shortcuts handles the few keys a desktop app is expected to know. They
// run before the view is built, so a page change lands in this frame.
func (a *App) shortcuts(c *ui.Context) {
	if a.phase != PhaseReady || a.onboardingRequired() {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.search.Focus(c)
	}
	targets := a.navTargets()
	for i, key := range navShortcuts {
		if i >= len(targets) {
			break
		}
		if c.Shortcut(ui.Cmd, key) {
			a.router.Push(targets[i])
			return
		}
	}
}

func (a *App) syncPage() {
	path := strings.TrimPrefix(a.router.Path(), "/")
	if i := strings.IndexByte(path, '/'); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		path = "recipes"
		if !a.pantryOff {
			path = "pantry"
		}
	}
	a.page = path
}

// post runs fn on the main thread and redraws, for work done on goroutines.
func (a *App) post(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}
