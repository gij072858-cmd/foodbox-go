package app

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
	"github.com/gij072858-cmd/foodbox-go/internal/services/season"
	"github.com/gij072858-cmd/foodbox-go/internal/videoproxy"
)

// recomputeRecipeView refreshes the list the recipe page reads. It runs
// when the data, the search, a facet, the seasonal filter or the sort
// changes, not while drawing. The version index is rebuilt in the same
// pass, so a row never groups dishes itself.
func (a *App) recomputeRecipeView() {
	a.rebuildRecipeVersions()
	q := strings.ToLower(strings.TrimSpace(a.query))
	f := a.recipeFilter
	s := &a.recipeEdit
	out := make([]domain.Recipe, 0, len(a.recipes))
	for _, r := range a.recipes {
		if f != "" && f != "全部" &&
			r.MainCategory != f && r.Region != f && r.Cuisine != f {
			continue
		}
		if s.mainFilter != "" && r.MainCategory != s.mainFilter {
			continue
		}
		if s.regionFilter != "" && r.Region != s.regionFilter {
			continue
		}
		if s.typeFilter != "" && r.Cuisine != s.typeFilter {
			continue
		}
		if q != "" && !recipeMatches(r, q) {
			continue
		}
		if !a.pantryOff && a.recipeAvail != "" &&
			string(a.recipeMatches[r.ID].Status) != a.recipeAvail {
			continue
		}
		if s.seasonOnly && !a.seasonMatch(r) {
			continue
		}
		out = append(out, r)
	}
	// The table's own sort wins while it has one; otherwise the card view's
	// four choices apply. Both are applied here, never inside a row.
	if col := a.recipeSort.Column; col != "" {
		desc := a.recipeSort.Descending
		sort.SliceStable(out, func(i, j int) bool {
			c := a.recipeCompare(col, out[i], out[j])
			if desc {
				return c > 0
			}
			return c < 0
		})
	} else {
		switch s.sortFilter {
		case 1:
			sort.SliceStable(out, func(i, j int) bool {
				return recipeDuration(out[i]) < recipeDuration(out[j])
			})
		case 2:
			sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
		case 3:
			sort.SliceStable(out, func(i, j int) bool {
				ki, _ := a.dishNut(out[i])
				kj, _ := a.dishNut(out[j])
				return ki.Nutrients.Kcal < kj.Nutrients.Kcal
			})
		}
	}
	a.recipeView = out
	a.recipeSelected = -1
}

// recipeCompare orders two dishes by one table column: negative when x
// comes first. Stock ranks by how close the dish is to cookable, so "can
// cook" sorts first ascending.
func (a *App) recipeCompare(column string, x, y domain.Recipe) int {
	switch column {
	case "duration":
		return cmp.Compare(recipeDuration(x), recipeDuration(y))
	case "kcal":
		kx, _ := a.dishNut(x)
		ky, _ := a.dishNut(y)
		return cmp.Compare(kx.Nutrients.Kcal, ky.Nutrients.Kcal)
	case "stock":
		return cmp.Compare(recipeStockRank(a.recipeMatches[x.ID].Status), recipeStockRank(a.recipeMatches[y.ID].Status))
	case "meta":
		return strings.Compare(recipeMeta(x), recipeMeta(y))
	default:
		return strings.Compare(x.Name, y.Name)
	}
}

func recipeStockRank(s cookmatch.Status) int {
	switch s {
	case cookmatch.CanCook:
		return 0
	case cookmatch.Almost:
		return 1
	}
	return 2
}

// recipeDuration is a dish's cooking time for sorting; unknown durations
// go last.
func recipeDuration(rec domain.Recipe) float64 {
	if rec.DurationMinutes == nil {
		return math.Inf(1)
	}
	return *rec.DurationMinutes
}

// seasonMatch reports whether a dish belongs in the 「当季」 filter: it must
// carry a real season window and the current month must fall in it. A
// dish sold all year has no season to be in, so it stays out.
func (a *App) seasonMatch(rec domain.Recipe) bool {
	return season.Marked(rec.SeasonMonths) && season.InSeason(rec.SeasonMonths, a.recipeEdit.seasonMonth)
}

func recipeMatches(r domain.Recipe, q string) bool {
	fields := []string{r.Name, r.MainCategory, r.Region, r.Cuisine, r.Intro}
	for _, in := range r.Ingredients {
		fields = append(fields, in.Name)
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// setRecipes publishes a fresh dish snapshot and rebuilds every view that
// derives from it. It runs on the main thread after a load or a write.
func (a *App) setRecipes(recipes []domain.Recipe, steps map[int64][]domain.Step) {
	a.recipes = recipes
	byID := make(map[int64]domain.Recipe, len(recipes))
	for _, r := range recipes {
		byID[r.ID] = r
	}
	a.recipeByID = byID
	a.steps = steps
	// A dish that left the database leaves the schedule with it: the
	// schedule_items row cascades, so the snapshot must not keep a ghost.
	kept := a.schedule[:0]
	for _, it := range a.schedule {
		if _, ok := byID[it.RecipeID]; ok {
			kept = append(kept, it)
		}
	}
	if len(kept) != len(a.schedule) {
		a.schedule = kept
		if _, ok := byID[a.scheduleSelected]; !ok {
			a.scheduleSelected = 0
		}
	}
	a.clearDishNutCache()
	a.computeRecipeMatches()
}

// reloadRecipes reads the dish snapshot back after a write and refreshes
// the views. With no window (headless tests) it runs inline so the caller
// sees the result at once; otherwise it runs on a goroutine.
func (a *App) reloadRecipes(after func()) {
	store := a.store
	if store == nil {
		if after != nil {
			after()
		}
		return
	}
	load := func() ([]domain.Recipe, map[int64][]domain.Step, error) {
		ctx := context.Background()
		recipes, err := store.ListAll(ctx)
		if err != nil {
			return nil, nil, err
		}
		steps, err := store.AllSteps(ctx)
		if err != nil {
			return nil, nil, err
		}
		return recipes, steps, nil
	}
	if a.win == nil {
		recipes, steps, err := load()
		if err != nil {
			a.notice = "读取菜式失败：" + err.Error()
			return
		}
		a.setRecipes(recipes, steps)
		if after != nil {
			after()
		}
		return
	}
	go func() {
		recipes, steps, err := load()
		a.post(func() {
			if err != nil {
				a.notice = "读取菜式失败：" + err.Error()
				return
			}
			a.setRecipes(recipes, steps)
			if after != nil {
				after()
			}
		})
	}()
}

// rebuildRecipeVersions indexes dishes by version group once per data or
// filter change. Rows only look up the precomputed counts.
func (a *App) rebuildRecipeVersions() {
	versions := make(map[string][]int64, 128)
	groupOf := make(map[int64]string, len(a.recipes))
	for _, r := range a.recipes {
		group := domain.VersionGroupOf(r)
		versions[group] = append(versions[group], r.ID)
		groupOf[r.ID] = group
	}
	a.recipeEdit.versions = versions
	a.recipeEdit.versionOf = groupOf
}

// versionCount returns how many dishes share rec's version group; 1 means
// this dish stands alone.
func (a *App) versionCount(rec domain.Recipe) int {
	return len(a.recipeEdit.versions[a.recipeEdit.versionOf[rec.ID]])
}

// versionMembers returns the dishes sharing rec's version group, in seed
// order, the dish itself included.
func (a *App) versionMembers(rec domain.Recipe) []domain.Recipe {
	ids := a.recipeEdit.versions[a.recipeEdit.versionOf[rec.ID]]
	out := make([]domain.Recipe, 0, len(ids))
	for _, id := range ids {
		if member, ok := a.recipeByID[id]; ok {
			out = append(out, member)
		}
	}
	return out
}

func (a *App) recipesPage(c *ui.Context, p design.Palette) {
	// A table header changes the sort itself; re-derive the rows before
	// building them.
	if a.recipeSort != a.recipeEdit.lastTableSort {
		a.recipeEdit.lastTableSort = a.recipeSort
		a.recomputeRecipeView()
	}
	if !reflect.DeepEqual(a.recipeTable.Columns, a.recipeEdit.savedColumns) {
		a.recipeEdit.savedColumns = a.recipeTable.Columns
		a.savePrefs()
	}
	ui.Column(c).Grow(1).FillWidth().MinWidth(0).Children(func() {
		a.pageFrame(c, func() {
			// Grow+MinHeight(0) bound the table/grid to the page, so their
			// virtualized lists see a viewport and build only the visible
			// rows. An auto-height column made the list expand to all 944
			// rows, which cost ~40 ms on every frame — including the frames
			// the sidebar pill slides over.
			ui.Column(c).Grow(1).FillWidth().MinHeight(0).Gap(design.Sp(c, 2)).
				Padding(0, 0, design.Sp(c, 2.5), 0).Children(func() {
				// The page's one action sits in the content, not in the
				// window's top strip: that strip is drag room only.
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("%d 道菜", len(a.recipeView))).
						FontSize(design.FontMeta).TextColor(p.Muted)
					ui.Box(c).Grow(1)
					if design.Button(c, p, "top-new-recipe", "新建菜式",
						design.ButtonPrimary).Clicked() {
						a.openRecipeEditor(0)
					}
				})
				a.recipeFilterBar(c, p)
				if len(a.recipeView) == 0 {
					empty := design.EmptyState(c, p, &icons.Recipe, "没有匹配的菜式",
						"换个关键词、取消当季筛选，或点「全部」看看全部菜式。")
					empty.Margin(design.PagePadding, 0, 0, 0)
					empty.Children(func() {
						if design.Button(c, p, "recipe-clear", "清除筛选", design.ButtonSecondary).Clicked() {
							a.clearRecipeFilters()
						}
					})
					return
				}
				a.recipeTableView(c, p)
			})
		})
	})
}

// recipeFilterBar is one row: stock segments, the four limits as menus or
// selects, and the card/table switch. Narrow windows keep the low-frequency
// two behind a 「筛选」 menu instead of scrolling sideways.
func (a *App) recipeFilterBar(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	idx := recipeStockIndex(a.recipeAvail)
	labels := []string{
		"全部",
		fmt.Sprintf("能做(%d)", a.canCook),
		fmt.Sprintf("差一点(%d)", a.almost),
		fmt.Sprintf("差太多(%d)", a.farAway),
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(design.Sp(c, 1)).Children(func() {
		if !a.pantryOff {
			if design.Segmented(c, &idx, labels...).Changed() {
				a.recipeAvail = recipeStockNames[idx]
				a.recomputeRecipeView()
			}
		}
		ui.Box(c).Grow(1)
		// The page column's room, from the window: 1280 leaves 968, 1024
		// leaves 712.
		compact := a.winW-design.SidebarWidth-2*design.PagePadding < 940
		if compact {
			a.recipeFilterMenu(c, p)
		} else {
			if selectFilter(c, &s.mainFilter, "主料不限", recipeMainCategoryOptions) {
				a.recomputeRecipeView()
			}
			if selectFilter(c, &s.regionFilter, "菜系不限", recipeRegionOptions) {
				a.recomputeRecipeView()
			}
			if selectFilter(c, &s.typeFilter, "类型不限", recipeCuisineOptions) {
				a.recomputeRecipeView()
			}
		}
		sortLabels := []string{"默认排序", "最快", "最新", "热量"}
		sort := sortLabels[clampInt(s.sortFilter, 0, len(sortLabels)-1)]
		if ui.Select(c, &sort, sortLabels).Width(112).MinWidth(0).Changed() {
			for i, label := range sortLabels {
				if label == sort {
					s.sortFilter = i
				}
			}
			a.recomputeRecipeView()
		}
		season := s.seasonOnly
		tip := fmt.Sprintf("本月 %d 月", s.seasonMonth)
		if ui.Toggle(c, &season, "当季").Tooltip(tip).Changed() {
			s.seasonOnly = season
			a.recomputeRecipeView()
		}
	})
}

// recipeFilterMenu keeps the three limits reachable when the window is too
// narrow for another select.
func (a *App) recipeFilterMenu(c *ui.Context, p design.Palette) {
	ui.PopoverMenuButton(c, "筛选", func(m *ui.Menu) {
		m.Submenu("主料", func(sub *ui.Menu) {
			if sub.Item("主料不限").Chosen() {
				a.recipeEdit.mainFilter = ""
				a.recomputeRecipeView()
			}
			for _, opt := range recipeMainCategoryOptions {
				opt := opt
				if sub.Item(opt).Checked(a.recipeEdit.mainFilter == opt).Chosen() {
					a.recipeEdit.mainFilter = opt
					a.recomputeRecipeView()
				}
			}
		})
		m.Submenu("菜系", func(sub *ui.Menu) {
			if sub.Item("菜系不限").Chosen() {
				a.recipeEdit.regionFilter = ""
				a.recomputeRecipeView()
			}
			for _, opt := range recipeRegionOptions {
				opt := opt
				if sub.Item(opt).Checked(a.recipeEdit.regionFilter == opt).Chosen() {
					a.recipeEdit.regionFilter = opt
					a.recomputeRecipeView()
				}
			}
		})
		m.Submenu("类型", func(sub *ui.Menu) {
			if sub.Item("类型不限").Chosen() {
				a.recipeEdit.typeFilter = ""
				a.recomputeRecipeView()
			}
			for _, opt := range recipeCuisineOptions {
				opt := opt
				if sub.Item(opt).Checked(a.recipeEdit.typeFilter == opt).Chosen() {
					a.recipeEdit.typeFilter = opt
					a.recomputeRecipeView()
				}
			}
		})
	})
}

// recipeStockNames are the availability choices, in the segment order.
var recipeStockNames = []string{"", string(cookmatch.CanCook), string(cookmatch.Almost), string(cookmatch.FarAway)}

func recipeStockIndex(status string) int {
	for i, name := range recipeStockNames {
		if name == status {
			return i
		}
	}
	return 0
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// selectFilter is a Select whose first choice means "no limit": the value
// stays an empty string for it, so the recompute has one meaning for all
// four filters.
func selectFilter(c *ui.Context, value *string, all string, options []string) bool {
	labels := append([]string{all}, options...)
	current := all
	if *value != "" {
		current = *value
	}
	if !ui.Select(c, &current, labels).Width(112).MinWidth(0).Changed() {
		return false
	}
	if current == all {
		*value = ""
	} else {
		*value = current
	}
	return true
}

func (a *App) clearRecipeFilters() {
	s := &a.recipeEdit
	a.query = ""
	a.recipeFilter = ""
	a.recipeAvail = ""
	s.seasonOnly = false
	s.mainFilter = ""
	s.regionFilter = ""
	s.typeFilter = ""
	s.sortFilter = 0
	a.recipeSort = ui.SortOrder{}
	a.recomputeRecipeView()
}

// recipeTableView is the compact mode: the same rows as a sortable table,
// without covers.
func (a *App) recipeTableView(c *ui.Context, p design.Palette) {
	cols := []ui.TableColumn{
		{Title: "菜式", ID: "name", Sortable: true, MinWidth: 180},
		{Title: "菜系 · 分类", ID: "meta", Sortable: true, Width: 160},
		{Title: "耗时", ID: "duration", Sortable: true, Width: 80, Align: ui.End},
		{Title: "热量", ID: "kcal", Sortable: true, Width: 80, Align: ui.End},
	}
	stockCol := -1
	if !a.pantryOff {
		stockCol = len(cols)
		cols = append(cols, ui.TableColumn{Title: "库存", ID: "stock", Width: 110})
	}
	tbl := ui.Table(c, &a.recipeTable, cols, len(a.recipeView), func(row, col int) {
		rec := a.recipeView[row]
		switch col {
		case 0:
			ui.Text(c, rec.Name).FontSize(design.FontSmall).FontWeight(600).
				TextColor(p.Text).SingleLine()
		case 1:
			ui.Text(c, recipeMeta(rec)).FontSize(design.FontMeta).TextColor(p.Muted).SingleLine()
		case 2:
			ui.Text(c, rec.DurationText()).FontSize(design.FontMeta).
				TextColor(p.Muted).TextAlign(ui.End)
		case 3:
			nut, ok := a.dishNut(rec)
			label := "—"
			if ok && nut.Nutrients.Kcal > 0 {
				label = fmt.Sprintf("%.0f", nut.Nutrients.Kcal)
			}
			ui.Text(c, label).FontSize(design.FontMeta).TextColor(p.Muted).TextAlign(ui.End)
		default:
			if col == stockCol {
				a.stockBadge(c, p, rec)
			}
		}
	})
	// The table chooses a row on a click and on the arrow keys. Only a click
	// while the pointer is over the table opens the dish; Enter and a double
	// click (Submitted) keep the row reachable from the keyboard.
	if (tbl.Changed() && tbl.Hovered()) || tbl.Submitted() {
		if a.recipeSelected >= 0 && a.recipeSelected < len(a.recipeView) {
			rec := a.recipeView[a.recipeSelected]
			a.recipeSelected = -1
			a.router.Push(fmt.Sprintf("/recipes/%d", rec.ID))
		}
	}
	tbl.Grow(1).FillWidth().MinWidth(0).
		Padding(design.PagePadding, 0, design.PagePadding, 0)
}

// stockBadge is the list's one-word answer to "can I cook this now".
func (a *App) stockBadge(c *ui.Context, p design.Palette, rec domain.Recipe) {
	res := a.recipeMatches[rec.ID]
	switch res.Status {
	case cookmatch.CanCook:
		design.Tag(c, p, "能做", p.Success)
	case cookmatch.Almost:
		design.Tag(c, p, "差 "+fmt.Sprint(len(res.Missing))+" 样", p.Warning)
	default:
		design.Tag(c, p, "差太多", p.Faint)
	}
}

// ingredientTable is the dish's ingredient list as a table: the framework
// gives the rows roles, keyboard cell browsing and a header, which a stack
// of hand-built rows never had.
func (a *App) ingredientTable(c *ui.Context, p design.Palette, rec domain.Recipe) {
	missing := make(map[string]bool, len(a.recipeMatches[rec.ID].Missing))
	for _, in := range a.recipeMatches[rec.ID].Missing {
		missing[in.Name] = true
	}
	cols := []ui.TableColumn{
		{Title: "食材", ID: "name", MinWidth: 140},
		{Title: "用量", ID: "amount", Width: 130, Align: ui.End},
	}
	stockCol := -1
	if !a.pantryOff {
		stockCol = len(cols)
		cols = append(cols, ui.TableColumn{Title: "在库", ID: "stock", Width: 90})
	}
	kindCol := len(cols)
	cols = append(cols, ui.TableColumn{Title: "类型", ID: "kind", Width: 80})
	height := float32(30*len(rec.Ingredients) + 34)
	ui.Table(c, &a.ingredientList, cols, len(rec.Ingredients), func(row, col int) {
		in := rec.Ingredients[row]
		switch {
		case col == 0:
			ui.Text(c, in.Name).FontSize(design.FontSmall).TextColor(p.Text).SingleLine()
		case col == 1:
			amount, grams := ingredientDetail(in)
			if grams != "" {
				amount += " · " + grams
			}
			ui.Text(c, amount).FontSize(design.FontMeta).TextColor(p.Muted).TextAlign(ui.End)
		case col == stockCol:
			if missing[in.Name] {
				design.Tag(c, p, "缺", p.Warning)
			} else {
				design.Tag(c, p, "有", p.Success)
			}
		case col == kindCol:
			switch {
			case in.Seasoning:
				ui.Text(c, "调料").FontSize(design.FontMeta).TextColor(p.Faint)
			case in.Required:
				ui.Text(c, "必需").FontSize(design.FontMeta).TextColor(p.Muted)
			default:
				ui.Text(c, "可选").FontSize(design.FontMeta).TextColor(p.Faint)
			}
		}
	}).FillWidth().Height(height)
}

func recipeMeta(rec domain.Recipe) string {
	parts := make([]string, 0, 3)
	if rec.MainCategory != "" {
		parts = append(parts, rec.MainCategory)
	}
	if rec.Region != "" {
		parts = append(parts, rec.Region)
	} else if rec.Cuisine != "" {
		parts = append(parts, rec.Cuisine)
	}
	if d := rec.DurationText(); d != "" {
		parts = append(parts, d)
	}
	return strings.Join(parts, " · ")
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

func (a *App) recipeDetail(c *ui.Context, r *ui.Route, p design.Palette) {
	id, _ := strconv.ParseInt(r.Param("id"), 10, 64)
	rec, ok := a.recipeByID[id]
	if !ok {
		a.pageBody(c, func() {
			ui.Text(c, "菜式不存在").FontSize(22).FontWeight(700).TextColor(p.Text)
			if ui.Button(c, "回到菜式").Clicked() {
				a.router.Replace("/recipes")
			}
		})
		return
	}
	page := ui.Column(c).Grow(1).FillWidth().MinWidth(0)
	page.Children(func() {
		a.pageBody(c, func() {
			ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).
				Margin(0, 0, 18, 0).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "编辑").Clicked() {
						a.openRecipeEditor(rec.ID)
					}
					if design.Button(c, p, "recipe-delete", "删除菜式",
						design.ButtonDanger).Clicked() {
						a.askDeleteRecipe(rec.ID)
					}
				})
			})

			ui.Row(c).FillWidth().Gap(24).MinWidth(0).Children(func() {
				a.recipeHeroCard(c, p, rec)
				ui.Column(c).Grow(1).MinWidth(0).Shrink(1).Gap(10).Justify(ui.Center).Children(func() {
					ui.Text(c, rec.Name).FontSize(32).FontWeight(700).TextColor(p.Text).Role(ui.RoleHeading)
					if meta := recipeMeta(rec); meta != "" {
						ui.Text(c, meta).FontSize(13).TextColor(p.Muted)
					}
					if rec.Intro != "" {
						ui.Text(c, rec.Intro).FontSize(13).TextColor(p.Muted).LineHeight(1.5)
					}
					if rec.Source != nil && rec.Source.URL != "" {
						ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
							// The label says which of the two paths the click
							// takes: a resolved direct stream, or this page
							// inside the app because the site has no endpoint
							// to resolve.
							videoLabel := "看视频"
							if !videoproxy.Supported(rec.Source.URL) {
								videoLabel = "在应用内看视频"
							}
							if design.Button(c, p, "recipe-video", videoLabel,
								design.ButtonSecondary).Clicked() {
								a.openVideo(c, rec.Source.URL)
							}
							label := "在浏览器打开"
							if rec.Source.Platform != "" {
								label = "在" + rec.Source.Platform + "打开"
							}
							ui.Link(c, label, rec.Source.URL).FontSize(design.FontSmall)
						})
					}
					// The hero owns the numbers that need its width; these are
					// the quick facts that read beside the title.
					ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
						design.Pill(c, p, fmt.Sprintf("%d 步", len(a.steps[rec.ID])))
						design.Pill(c, p, difficultyText(rec.Difficulty))
						a.seasonTag(c, p, rec)
					})
					if !a.pantryOff {
						res := a.recipeMatches[rec.ID]
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							color := p.Success
							if res.Status == cookmatch.Almost {
								color = p.Warning
							} else if res.Status == cookmatch.FarAway {
								color = p.Faint
							}
							ui.Text(c, res.Status.Label()).FontSize(12).FontWeight(700).TextColor(color)
							if label := res.MissingLabel(); label != "" {
								ui.Text(c, label).FontSize(12).TextColor(p.Muted)
								if ui.Button(c, "加入购物清单").Clicked() {
									a.addMissingToShopping(rec)
								}
							}
						})
					}
					ui.Row(c).Gap(10).Margin(6, 0, 0, 0).Children(func() {
						a.primaryButton(c, "加入掌序", func() {
							a.addToSchedule(rec)
						})
						if ui.Button(c, "收藏").Clicked() {
							a.toggleFavorite(rec)
						}
						if ui.Button(c, "打开掌序演示").Clicked() {
							a.router.Push("/schedule")
						}
					})
				})
			})

			if members := a.versionMembers(rec); len(members) > 1 {
				a.section(c, p, fmt.Sprintf("%d 种做法", len(members)), func() {
					labels := make([]string, 0, len(members))
					for _, member := range members {
						label := domain.VariantLabelOf(member)
						if label == "" {
							label = member.Name
						}
						labels = append(labels, label)
					}
					pick := domain.VariantLabelOf(rec)
					if pick == "" {
						pick = rec.Name
					}
					if ui.Select(c, &pick, labels).Width(240).Changed() {
						for _, member := range members {
							label := domain.VariantLabelOf(member)
							if label == "" {
								label = member.Name
							}
							if label == pick && member.ID != rec.ID {
								a.router.Push(fmt.Sprintf("/recipes/%d", member.ID))
							}
						}
					}
					ui.Text(c, "同一个版本组下的不同做法，选一项即切换。").FontSize(11).TextColor(p.Faint)
				})
			}

			if len(rec.Ingredients) > 0 {
				a.section(c, p, "食材", func() {
					a.ingredientTable(c, p, rec)
				})
			}

			a.section(c, p, "步骤", func() {
				steps := a.steps[rec.ID]
				if len(steps) == 0 {
					ui.Text(c, "这道菜还没有步骤。").FontSize(13).TextColor(p.Muted)
					if ui.Button(c, "补上步骤").Clicked() {
						a.openRecipeEditor(rec.ID)
					}
					return
				}
				for i, s := range steps {
					i, s := i, s
					ui.Row(c).FillWidth().Gap(12).Padding(12, 14).Radius(12).
						Background(p.Surface).Border(1, p.Border).Children(func() {
						ui.Text(c, fmt.Sprint(i+1)).FontSize(12).FontWeight(700).TextColor(p.Accent).
							Width(24).Height(24).TextAlign(ui.Center).Radius(999).Background(p.AccentPale)
						ui.Column(c).Grow(1).Gap(3).Children(func() {
							ui.Text(c, s.Instruction).FontSize(13).TextColor(p.Text).LineHeight(1.5)
							ui.Row(c).Gap(10).Children(func() {
								if s.CompletionCue != "" {
									ui.Text(c, "完成标志："+s.CompletionCue).FontSize(11).TextColor(p.Muted)
								}
								if resources := resourceText(s.Resources); resources != "" {
									ui.Text(c, "占用："+resources).FontSize(11).TextColor(p.Faint)
								}
							})
						})
						if s.DurationMinutes != nil {
							ui.Text(c, trimFloat(*s.DurationMinutes)+" 分钟").FontSize(11).TextColor(p.Faint)
						}
					})
				}
			})
		})
		a.recipeStickyHeader(c, p, rec)
		a.recipeDeleteDialog(c, p)
	})
}

// recipeStickyHeader appears once the hero has scrolled away: a small
// cover, the name and the one action the page exists for. It fades and
// lifts in, and passes input through while hidden.
func (a *App) recipeStickyHeader(c *ui.Context, p design.Palette, rec domain.Recipe) {
	s := a.pageScroll()
	// The hero is 220 tall; once most of it is gone, the bar takes over.
	show := s.Y > 120
	bar := ui.Row(c.Key("recipe-sticky")).Absolute().Top(0).Left(0).Right(0).Height(56).
		Padding(0, design.PagePadding).Gap(12).AlignItems(ui.Center).
		Material(design.BarMaterial(p)).Shadow(0, 8, 20, -12, p.Shadow).
		Label("菜式迷你头部").Opacity(0)
	// Hidden, the bar must not sit over the breadcrumb's actions.
	bar.IgnoreInput(!show)
	shown := ui.Local(bar, "shown", func() bool { return false })
	if shown == nil {
		return
	}
	if show && !*shown {
		bar.Jump("in", 0)
	}
	*shown = show
	k := bar.AnimateTo("in", boolFloat(show), design.Pane)
	if k <= 0.001 {
		return
	}
	bar.Opacity(k).Translate(0, -8*(1-k))
	bar.Children(func() {
		ui.BackButton(c, a.router)
		a.recipeMarkTile(c, p, rec, 26)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, rec.Name).FontSize(design.FontBody).FontWeight(600).
				TextColor(p.Text).SingleLine()
			ui.Text(c, recipeMeta(rec)).FontSize(design.FontMeta).
				TextColor(p.Muted).SingleLine()
		})
		if design.Button(c, p, "sticky-add", "加入掌序", design.ButtonPrimary).Clicked() {
			a.addToSchedule(rec)
		}
		ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(1).Background(p.Border)
	})
}

// boolFloat is 1 for true and 0 for false, for AnimateTo targets.
func boolFloat(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// seasonTag shows the dish's season window: highlighted while the current
// month is in it, grey when it is out of season, plainly 「全年」 otherwise.
func (a *App) seasonTag(c *ui.Context, p design.Palette, rec domain.Recipe) {
	label := season.Label(rec.SeasonMonths)
	if !season.Marked(rec.SeasonMonths) {
		design.Pill(c, p, label)
		return
	}
	inSeason := season.InSeason(rec.SeasonMonths, a.recipeEdit.seasonMonth)
	color := p.Faint
	text := label + " · 不当季"
	if inSeason {
		color = p.Success
		text = label + " · 当季"
	}
	design.Tag(c, p, text, color)
}

func ingredientAmount(in domain.Ingredient) string {
	if in.Grams != nil && in.Unit == "" {
		return trimFloat(*in.Grams) + " g"
	}
	if in.Amount == 0 && in.Unit == "" {
		return "适量"
	}
	return trimFloat(in.Amount) + " " + in.Unit
}

// ingredientDetail returns the amount text and, separately, the gram
// equivalent when the unit is not already grams.
func ingredientDetail(in domain.Ingredient) (amount, grams string) {
	amount = ingredientAmount(in)
	if in.Grams != nil && strings.TrimSpace(in.Unit) != "g" && in.Unit != "" {
		grams = "约 " + trimFloat(*in.Grams) + " g"
	}
	return amount, grams
}

func trimFloat(v float64) string {
	return domain.TrimFloat(v)
}

func difficultyText(d domain.Difficulty) string {
	switch d {
	case domain.DifficultyMedium:
		return "难度 中等"
	case domain.DifficultyHard:
		return "难度 困难"
	default:
		return "难度 简单"
	}
}

// resourceText names a step's resources in the order the editor shows.
func resourceText(resources []string) string {
	if len(resources) == 0 {
		return ""
	}
	has := make(map[string]bool, len(resources))
	for _, r := range resources {
		has[r] = true
	}
	names := make([]string, 0, len(resources))
	for _, opt := range stepResourceOptions {
		if has[opt.Name] {
			names = append(names, opt.Label)
		}
	}
	return strings.Join(names, " · ")
}

// addToSchedule is the M1 bridge to the schedule page. M4 replaces it with
// the repository-backed schedule engine; the view still only calls the app.
func (a *App) addToSchedule(rec domain.Recipe) {
	for _, it := range a.schedule {
		if it.RecipeID == rec.ID {
			a.notice = "这道菜已经在掌序里"
			return
		}
	}
	a.schedule = append(a.schedule, domain.ScheduleItem{
		RecipeID: rec.ID, AddedAt: time.Now(), Servings: rec.BaseServings,
		Status: domain.SchedulePending,
	})
	a.notice = "已加入掌序：" + rec.Name
	if a.scheduleSelected == 0 {
		a.scheduleSelected = rec.ID
		a.lyricFocus = 0
		a.lyric.Y = lyricCenterOffset(0, 430)
	}
	a.computeShopping()
	a.persistScheduleAdd(rec.ID, rec.BaseServings, "")
}

func (a *App) toggleFavorite(rec domain.Recipe) {
	a.favorites[rec.ID] = !a.favorites[rec.ID]
	// Flip the map at once so the card reacts this frame, then make it
	// durable; the profile page reconciles on entry as a safety net.
	a.persistFavorite(rec.ID, a.favorites[rec.ID])
}
