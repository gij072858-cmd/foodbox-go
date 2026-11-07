package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
)

// sectionTitles name the first path segment: the breadcrumb's first crumb
// and the root pages' heading.
var sectionTitles = map[string]string{
	"pantry": "仓库", "recipes": "菜式", "sichu": "私厨", "schedule": "掌序",
	"feed": "食历", "nutrition": "营养驾驶舱", "profile": "我的",
	"settings": "设置", "help": "帮助", "onboarding": "开始使用", "probe": "动效探针",
}

// leafTitles name the pages below a section. Dynamic pages (a dish, a day)
// read the title the route declared, because the id is not a word.
var leafTitles = map[string]string{
	"/pantry/catalog":    "系统食材库",
	"/pantry/starter":    "新手包",
	"/pantry/shopping":   "购物清单",
	"/pantry/reconcile":  "盘点核销",
	"/recipes/new":       "新建菜式",
	"/sichu/result":      "私厨配餐",
	"/sichu/body":        "身体档案",
	"/sichu/members":     "同桌成员",
	"/schedule/prep":     "备料清单",
	"/schedule/timeline": "排程时间线",
	"/schedule/run":      "掌序执行",
	"/feed/review":       "年度回顾",
	"/profile/edit":      "编辑资料",
	"/profile/cooked":    "做过的菜",
	"/profile/shelf":     "我的收藏",
	"/settings/legacy":   "导入旧版数据库",
	"/settings/ai":       "AI 与配额",
}

// routeDepth counts the path's segments. The framework's Router has no
// depth query, and the shell only needs this one fact.
func routeDepth(path string) int {
	n := 0
	for _, seg := range strings.Split(path, "/") {
		if seg != "" {
			n++
		}
	}
	return n
}

// routeDirection is the way a page change goes: deeper into a section,
// back up, or across between sections at the same depth.
func routeDirection(from, to int) int {
	switch {
	case to > from:
		return 1
	case to < from:
		return -1
	default:
		return 0
	}
}

// routeLeafTitle is the page's own name: the static table where it has
// one, the route's declared title otherwise (a dish, a day, an editor).
func (a *App) routeLeafTitle(path string) string {
	if leaf, ok := leafTitles[path]; ok {
		return leaf
	}
	first := strings.TrimPrefix(path, "/")
	if i := strings.IndexByte(first, '/'); i >= 0 {
		first = first[:i]
	}
	if first == "" {
		first = "recipes"
	}
	if title, ok := sectionTitles[first]; ok {
		if routeDepth(path) == 1 {
			return title
		}
	}
	switch {
	case strings.HasPrefix(path, "/recipes/") && strings.HasSuffix(path, "/edit"):
		return "编辑菜式"
	case strings.HasPrefix(path, "/recipes/"):
		return "菜式详情"
	case strings.HasPrefix(path, "/feed/"):
		return "开饭日记"
	}
	return "食匣"
}

// breadcrumbItems pairs each crumb's label with the path it opens. Only
// pages that exist are offered, so a crumb never pushes a dead route.
func (a *App) breadcrumbItems() ([]string, []string) {
	path := a.router.Path()
	if i := strings.IndexByte(path[1:], '?'); i >= 0 {
		path = path[:i+1]
	}
	first := strings.TrimPrefix(path, "/")
	if i := strings.IndexByte(first, '/'); i >= 0 {
		first = first[:i]
	}
	if first == "" {
		first = "recipes"
	}
	section, ok := sectionTitles[first]
	if !ok {
		section = a.routeLeafTitle(path)
	}
	labels := []string{section}
	paths := []string{"/" + first}
	if leaf := a.routeLeafTitle(path); leaf != section {
		labels = append(labels, leaf)
		paths = append(paths, path)
	}
	return labels, paths
}

func (a *App) sidebar(c *ui.Context, p design.Palette) {
	// The controls of a hidden title bar sit over the top of the window.
	// Where they are on the left — macOS' traffic lights — the brand starts
	// under them rather than behind them; elsewhere the strip is theirs to
	// drag the window from. Cloudy lays this out the same way (ui_main.go:196-202).
	top := float32(14)
	if bar := c.TitleBar(); bar.Left > 0 {
		top = bar.Height + 4
	}
	side := ui.Column(c).Width(design.SidebarWidth).Shrink(0).Padding(top, 12).
		Gap(10).DragWindow()
	side.Children(func() {
		// The mark is the name alone: no icon and no English caption over
		// the one word the app is called, so the search field rises to sit
		// just under it.
		ui.Row(c).AlignItems(ui.Center).Padding(4, 6).Children(func() {
			ui.Text(c, "食匣").FontSize(18).FontWeight(700).TextColor(p.Text)
		})

		placeholder, searchLabel := "搜索菜式、食材", "搜索菜式与食材"
		if a.pantryOff {
			placeholder, searchLabel = "搜索菜式", "搜索菜式"
		}
		if ui.SearchField(c, &a.query).Bind(&a.search).
			Placeholder(placeholder).Label(searchLabel).Changed() {
			a.recomputeRecipeView()
			// The field searches dishes, so typing anywhere takes the
			// result where it is shown instead of leaving it invisible.
			if a.query != "" && a.page != "recipes" {
				a.router.Push("/recipes")
			}
		}

		// The sidebar is built here rather than with ui.Sidebar: the
		// framework's item changes its face at once, while Cloudy's pill
		// slides. Match is the framework's matched geometry, so the slide
		// is the same primitive, not a second animation engine.
		nav := ui.Column(c).Grow(1).MinWidth(0).Gap(14).Role(ui.RoleTree).Focusable()
		a.navKeys(nav)
		nav.Children(func() {
			// The three shelves are told apart by a hairline, not by
			// headings: the destinations are the content, and a label over
			// four short rows only added a second line of grammar.
			kitchen := []navItem{
				{"recipes", &icons.Recipe, "菜式", 0},
				{"sichu", &icons.Sichu, "私厨", 0},
				{"schedule", &icons.Schedule, "掌序", len(a.schedule)},
			}
			if !a.pantryOff {
				kitchen = append([]navItem{{"pantry", &icons.Pantry, "仓库", len(a.items)}}, kitchen...)
			}
			a.navSection(c, p, kitchen)
			a.navRule(c)
			a.navSection(c, p, []navItem{
				{"feed", &icons.Feed, "食历", 0},
				{"nutrition", &icons.Chart, "营养", 0},
				{"profile", &icons.Profile, "我的", 0},
			})
			a.navRule(c)
			a.navSection(c, p, []navItem{
				{"help", &icons.Help, "帮助", 0},
				{"settings", &icons.Settings, "设置", 0},
			})
		})
	})
}

// navItem is one sidebar destination.
type navItem struct {
	id    string
	icon  *ui.SVG
	label string
	badge int
}

// navPillMatch identifies the selected pill across items, so Match flies
// it from the item chosen before.
type navPillMatch struct{}

func (a *App) navSection(c *ui.Context, p design.Palette, items []navItem) {
	ui.Column(c).FillWidth().Gap(2).FocusGroup(ui.Vertical).Children(func() {
		for _, it := range items {
			a.navItem(c, p, it)
		}
	})
}

// navRule is the hairline between two shelves of destinations.
func (a *App) navRule(c *ui.Context) {
	ui.Divider(c).Opacity(0.6).Margin(6, 8, 6, 8)
}

func (a *App) navItem(c *ui.Context, p design.Palette, it navItem) {
	selected := a.page == it.id
	b := ui.ButtonBase(c.Key("nav-" + it.id)).FillWidth().Height(34).
		Radius(design.RadiusSmall).Label(it.label).Role(ui.RoleTreeItem)
	if !selected {
		design.Wash(b, p.Sidebar.Alpha(0), p.Hover)
	}
	b.Children(func() {
		// The pill is in the flow, so it paints before the content; the
		// content is absolute over it. An absolute pill would paint last
		// and cover the label.
		if selected {
			a.navPill(c, p)
		}
		row := ui.Row(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Padding(6, 10).Gap(10).AlignItems(ui.Center).PassThrough()
		row.Children(func() {
			color := p.Ink
			if selected {
				color = p.OnAccent
			}
			ui.Icon(c, it.icon).Size(17, 17).TextColor(color)
			ui.Text(c, it.label).FontSize(design.FontBody).FontWeight(500).
				TextColor(color).Grow(1)
			if it.badge > 0 {
				// The count is a badge, not part of the label: a screen
				// reader still hears the destination's name alone.
				ui.Badge(c, fmt.Sprint(it.badge)).FontSize(design.FontCaption).
					FontWeight(700).Padding(1, 6).Radius(design.RadiusPill).
					Background(p.AccentPale).TextColor(p.Accent)
			}
		})
	})
	if b.Clicked() && !selected {
		a.page = it.id
		a.router.Push("/" + it.id)
	}
}

// navLetters are the letter keys, in typing order: the index in this slice
// is the letter (0 is a).
var navLetters = []ui.Key{
	ui.KeyA, ui.KeyB, ui.KeyC, ui.KeyD, ui.KeyE, ui.KeyF, ui.KeyG, ui.KeyH,
	ui.KeyI, ui.KeyJ, ui.KeyK, ui.KeyL, ui.KeyM, ui.KeyN, ui.KeyO, ui.KeyP,
	ui.KeyQ, ui.KeyR, ui.KeyS, ui.KeyT, ui.KeyU, ui.KeyV, ui.KeyW, ui.KeyX,
	ui.KeyY, ui.KeyZ,
}

// navKeys gives the sidebar tree its letters. They are element shortcuts,
// so they only fire while the keyboard is inside the tree: a letter typed
// into the search field above it is a letter, never a destination.
func (a *App) navKeys(nav ui.Element) {
	home := "pantry"
	if a.pantryOff {
		home = "recipes"
	}
	nav.OnShortcut(0, ui.KeyHome, func() { a.navGo(home) })
	nav.OnShortcut(0, ui.KeyEnd, func() { a.navGo("settings") })
	for i, key := range navLetters {
		letter := string(rune('a' + i))
		key := key
		nav.OnShortcut(0, key, func() { a.navTypeahead(letter) })
	}
}

// navTypeaheadEntry is one destination's pinyin, sorted so one letter has
// a stable first match: c goes to 菜式 (caishi) before 仓库 (cangku).
type navTypeaheadEntry struct {
	id, label, pinyin string
}

var navTypeaheadIndex = []navTypeaheadEntry{
	{"pantry", "仓库", "cangku"},
	{"recipes", "菜式", "caishi"},
	{"sichu", "私厨", "sichu"},
	{"schedule", "掌序", "zhangxu"},
	{"feed", "食历", "shili"},
	{"nutrition", "营养", "yingyang"},
	{"profile", "我的", "wode"},
	{"help", "帮助", "bangzhu"},
	{"settings", "设置", "shezhi"},
}

// navTypeahead jumps to the first destination whose pinyin starts with
// what was typed. Typing the same letter again walks the matches, the way
// a list's type-ahead does.
func (a *App) navTypeahead(letter string) {
	now := time.Now()
	if now.Sub(a.navTypeaheadAt) > 350*time.Millisecond {
		a.navTypeaheadText = ""
		a.navTypeaheadStep = 0
	}
	a.navTypeaheadAt = now
	if a.navTypeaheadText == letter {
		a.navTypeaheadStep++
	} else {
		a.navTypeaheadText += letter
		a.navTypeaheadStep = 0
	}
	matches := make([]navTypeaheadEntry, 0, 3)
	for _, e := range navTypeaheadIndex {
		if a.pantryOff && e.id == "pantry" {
			continue
		}
		if strings.HasPrefix(e.pinyin, a.navTypeaheadText) {
			matches = append(matches, e)
		}
	}
	if len(matches) == 0 {
		a.navTypeaheadText = letter
		for _, e := range navTypeaheadIndex {
			if a.pantryOff && e.id == "pantry" {
				continue
			}
			if strings.HasPrefix(e.pinyin, letter) {
				matches = append(matches, e)
			}
		}
		a.navTypeaheadStep = 0
	}
	if len(matches) == 0 {
		return
	}
	// The same letter again walks the matches in pinyin order.
	sort.Slice(matches, func(i, j int) bool { return matches[i].pinyin < matches[j].pinyin })
	a.navGo(matches[a.navTypeaheadStep%len(matches)].id)
}

// navGo selects a sidebar destination from the keyboard.
func (a *App) navGo(id string) {
	if a.page == id {
		return
	}
	a.page = id
	a.router.Push("/" + id)
}

// navPill is the face of the chosen item. It is a matched element, so the
// pill travels between items instead of two pills fading in and out.
func (a *App) navPill(c *ui.Context, p design.Palette) {
	pill := ui.Box(c.Key("nav-pill")).Fill().Radius(design.RadiusSmall).
		PassThrough().Background(p.Accent)
	if a.level.Full() {
		pill.Transition(ui.ElementTransition{Animation: &design.Pane, Match: navPillMatch{}})
	}
}

// topBar is the strip over the hidden title bar. At a section's root it is
// nothing but the window's drag room: the sidebar already names the page,
// and a title repeated here only crowded the caption buttons. Below a root
// it is the way back and the way up: back, forward, crumbs.
func (a *App) topBar(c *ui.Context) {
	bar := c.TitleBar()
	depth := routeDepth(a.router.Path())
	// A root is blank drag room: the sidebar is the way around. The one
	// exception is a forward page left after Back, which has no other way
	// back into it.
	if depth > 1 || a.router.CanGoForward() {
		ui.Row(c).FillWidth().Height(design.TitleBand).
			Padding(0, max(16, bar.Right+12), 0, design.PagePadding).
			AlignItems(ui.Center).Gap(8).Children(func() {
			ui.BackButton(c, a.router)
			ui.ForwardButton(c, a.router)
			if depth == 1 {
				return
			}
			crumbs, paths := a.breadcrumbItems()
			a.crumbChosen = len(crumbs) - 1
			if ui.Breadcrumbs(c, crumbs, &a.crumbChosen).Changed() {
				if a.crumbChosen >= 0 && a.crumbChosen < len(paths) &&
					paths[a.crumbChosen] != a.router.Path() {
					// Replace, not Back: the crumb jumps to a parent and
					// the history must not rewind through pages it never
					// showed.
					a.router.Replace(paths[a.crumbChosen])
				}
			}
		})
		return
	}
	// Only the empty stretch drags the window: a drag on the bar around a
	// button would swallow the click that button waits for.
	ui.Row(c).FillWidth().Height(design.TitleBand).
		Padding(0, max(16, bar.Right+12), 0, design.PagePadding).
		Children(func() {
			ui.Box(c).Grow(1).Height(design.TitleBand).DragWindow().Label("窗口拖拽区")
		})
}

// contentColumn is the page's bounded column. It is centred, so a maximised
// window grows the margins rather than stretching every card.
func (a *App) contentColumn(c *ui.Context, pad float32, fn func()) {
	ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(design.PageMaxWidth).MinWidth(0).
			Padding(0, pad).Children(fn)
	})
}

// pageFrame is the bounded, centred column a page with a virtualized list
// lives in. Header and list are children of the same column, so their left
// edges land on the same line at every window width.
func (a *App) pageFrame(c *ui.Context, fn func()) {
	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Justify(ui.Center).Children(func() {
		ui.Column(c).Grow(1).FillWidth().MaxWidth(design.PageMaxWidth).MinWidth(0).
			Padding(design.Sp(c, 4), design.PagePadding).Children(fn)
	})
}

// pageScroll is the scroll state of the page on screen. Each route keeps
// its own, so going back restores the place and the edge fades read the
// right offset.
func (a *App) pageScroll() *ui.ScrollState {
	key := a.buildingPath
	if key == "" {
		key = a.router.Path()
	}
	s := a.scrolls[key]
	if s == nil {
		s = &ui.ScrollState{}
		a.scrolls[key] = s
	}
	return s
}

// pageBody is the scrollable content area under the top bar. It fades the
// top and bottom edges only while content is under them, as AppKit does.
func (a *App) pageBody(c *ui.Context, fn func()) {
	p := design.PaletteFor(a.themeName, c.Theme().Dark)
	s := a.pageScroll()
	box := ui.Box(c).Grow(1).FillWidth().MinWidth(0)
	box.Children(func() {
		ui.Scroll(c).Fill().MinWidth(0).TrackScroll(s).Padding(16, 0, 28, 0).Children(func() {
			a.contentColumn(c, design.PagePadding, func() {
				fn()
			})
		})
		a.scrollEdge(c, p, s, true)
		a.scrollEdge(c, p, s, false)
	})
}

// scrollEdge is the soft mask at one end of the scroll. It stays out of
// the way when there is nothing to hide.
func (a *App) scrollEdge(c *ui.Context, p design.Palette, s *ui.ScrollState, top bool) {
	show := s.Y > 1
	if !top {
		show = s.MaxY > 0 && s.Y < s.MaxY-1
	}
	edge := ui.Box(c).Absolute().Left(0).Right(0).Height(26).IgnoreInput(true)
	if top {
		edge.Top(0)
	} else {
		edge.Bottom(0)
	}
	k := float32(0)
	if show {
		k = 1
	}
	k = edge.AnimateTo("edge", k, design.Hover)
	if k <= 0.001 {
		return
	}
	// MyGo's own scroll edge: it replays the background over the content
	// with the gradient macOS uses, rather than a hand-drawn fade. The
	// color is the wash's stop at that edge, so the mask never disagrees
	// with the gradient under it.
	bg := p.Bg
	if n := len(p.Wash.Stops); n > 0 {
		if top {
			bg = p.Wash.Stops[0].Color
		} else {
			bg = p.Wash.Stops[n-1].Color
		}
	}
	edge.Opacity(k).Material(glass.ScrollEdge{Background: bg, Bottom: !top})
}

// listBody is the virtualized list under a page header. It carries only
// vertical padding: the frame already owns the horizontal line.
func (a *App) listBody(c *ui.Context, key any, state *ui.ListState, rows int, row func(int)) {
	ui.List(c.Key(key), state, rows, row).Grow(1).FillWidth().MinWidth(0).
		Padding(design.PagePadding, 0, design.PagePadding, 0)
}

func (a *App) section(c *ui.Context, p design.Palette, title string, fn func()) {
	ui.Column(c).FillWidth().Gap(design.SectionGap(c)).
		Margin(0, 0, design.SectionLead(c), 0).Children(func() {
		ui.Text(c, title).FontSize(design.FontHeading).FontWeight(700).TextColor(p.Text).
			Role(ui.RoleHeading)
		fn()
	})
}

// card is a clickable surface that lifts under the pointer and gives as it
// is pressed, both drawn in paint-only frames.
func (a *App) card(c *ui.Context, p design.Palette, key any, title, sub string, accent bool, onClick func()) {
	card := ui.ButtonBase(c.Key(key)).Column().Gap(6).Padding(16).Radius(design.RadiusCard).
		Border(1, p.Border).Label(title)
	design.Wash(card, p.Surface, p.Hover)
	if card.Hovered() {
		design.HoverShadow(card, p)
	} else {
		design.RestShadow(card, p)
	}
	design.Lift(card, 3, 0.97)
	card.Children(func() {
		if accent {
			ui.Box(c).FillWidth().Height(4).Radius(design.RadiusPill).Background(p.Accent)
		}
		ui.Text(c, title).FontSize(design.FontSubhead).FontWeight(600).TextColor(p.Text).SingleLine()
		if sub != "" {
			ui.Text(c, sub).FontSize(design.FontSmall).TextColor(p.Muted).SingleLine()
		}
	})
	if card.Clicked() && onClick != nil {
		onClick()
	}
}

func (a *App) primaryButton(c *ui.Context, label string, onClick func()) {
	if ui.PrimaryButton(c, label).Clicked() {
		onClick()
	}
}
