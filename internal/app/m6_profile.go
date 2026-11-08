package app

// M6 · 我的：资料编辑、做过的菜、收藏与想要。
//
// profilePage (the overview) lives in profile_views.go; this file owns the
// account state, the persisted favourites and the sub-pages. The block lives
// inside App's prefs state: App carries only the feed and prefs blocks, and
// settings, account, onboarding and help are all M6's.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
)

// profileState is the account and its two list pages.
type profileState struct {
	profile domain.Profile
	loaded  bool

	// Edit form.
	editLoaded bool
	editName   string
	editAvatar string

	cooked cookedState
	shelf  shelfState
}

// cookedState is the cooked-dish page: filtered rows and their totals.
type cookedState struct {
	rows  []cookedRow
	stats cookedStats

	view       int // 0 list, 1 stats
	timeFilter int // 0 all, 1 this month, 2 last 90 days
	main       string
	region     string
	sortBy     int // 0 newest, 1 most cooked, 2 name, 3 duration

	mains   []string
	regions []string
	confirm bool

	// applied guards the aggregation so it runs when the data or a filter
	// changes, never on every frame.
	applied  bool
	stamp    int
	stampLen int
}

// cookedRow is one dish aggregated over the record history.
type cookedRow struct {
	RecipeID int64
	Name     string
	Count    int
	Last     time.Time
	Minutes  float64
	Main     string
	Region   string
}

// cookedStats is the totals view of the cooked page.
type cookedStats struct {
	Records int
	Recipes int
	Days    int
	Minutes int
	Months  [12]int
	Top     []cookedRow
}

// shelfState is the 收藏 / 想要 page.
type shelfState struct {
	tab   int // 0 favourites, 1 wanted
	query string
	view  []shelfRow
	found []shelfRow

	// sig guards the rebuild so it runs when the group, the counts or the
	// search change, never on every frame.
	sig string
}

// shelfRow is one dish on the shelf.
type shelfRow struct {
	RecipeID int64
	Name     string
	Sub      string
}

// XPLevel is the account level: every 100 XP is one level.
func XPLevel(xp int) int {
	if xp < 0 {
		return 1
	}
	return 1 + xp/100
}

// XPProgress is the fraction of the way to the next level.
func XPProgress(xp int) float32 {
	if xp < 0 {
		return 0
	}
	return float32(xp%100) / 100
}

// loadProfileState reads the account row and the questionnaire flag.
func (a *App) loadProfileState(ctx context.Context, store repository.Repository) error {
	p, err := store.GetProfile(ctx)
	if err != nil {
		return err
	}
	up, err := store.GetUserProfile(ctx)
	if err != nil {
		return err
	}
	onboarding := onboardingFrom(up)
	a.post(func() {
		a.prefs.profile.profile = p
		a.prefs.profile.loaded = true
		a.favorites = make(map[int64]bool, len(p.FavoriteRecipes))
		for _, id := range p.FavoriteRecipes {
			a.favorites[id] = true
		}
		a.prefs.onboarding = onboarding
	})
	return nil
}

// persistFavorite writes a favourite change: the card flips the map at once
// and this makes it durable in profiles.favorite_recipe_ids.
func (a *App) persistFavorite(id int64, on bool) {
	if a.favorites == nil {
		a.favorites = map[int64]bool{}
	}
	a.favorites[id] = on
	ids := make([]int64, 0, len(a.favorites))
	for rid, fav := range a.favorites {
		if fav {
			ids = append(ids, rid)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	a.prefs.profile.profile.FavoriteRecipes = ids
	store := a.store
	if store == nil {
		return
	}
	go func() {
		if err := store.SetFavoriteRecipeIDs(context.Background(), ids); err != nil {
			a.post(func() { a.notice = "收藏没有写入：" + err.Error() })
		}
	}()
}

// syncFavorites makes a favourite toggled on a recipe card durable. The
// recipe card flips the live map only (that page belongs to another
// workstream); opening 我的 or 收藏 reconciles the stored list with it. It
// writes only when the two differ.
func (a *App) syncFavorites() {
	p := &a.prefs.profile
	if !p.loaded {
		return
	}
	live := make([]int64, 0, len(a.favorites))
	for id, on := range a.favorites {
		if on {
			live = append(live, id)
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i] < live[j] })
	if sameIDs(live, p.profile.FavoriteRecipes) {
		return
	}
	p.profile.FavoriteRecipes = live
	store := a.store
	if store == nil {
		return
	}
	go func() {
		if err := store.SetFavoriteRecipeIDs(context.Background(), live); err != nil {
			a.post(func() { a.notice = "收藏没有写入：" + err.Error() })
		}
	}()
}

// sameIDs reports whether two sorted id lists are equal.
func sameIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// persistWanted writes a "want to cook" change.
func (a *App) persistWanted(id int64, on bool) {
	p := &a.prefs.profile
	ids := append([]int64{}, p.profile.WantedRecipes...)
	set := make(map[int64]bool, len(ids))
	for _, rid := range ids {
		set[rid] = true
	}
	if on {
		set[id] = true
	} else {
		delete(set, id)
	}
	ids = ids[:0]
	for rid := range set {
		ids = append(ids, rid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	p.profile.WantedRecipes = ids
	store := a.store
	if store == nil {
		return
	}
	go func() {
		if err := store.SetWantedRecipeIDs(context.Background(), ids); err != nil {
			a.post(func() { a.notice = "想要的菜没有写入：" + err.Error() })
		}
	}()
}

// openProfileEdit seeds the edit form from the account row.
func (a *App) openProfileEdit() {
	p := &a.prefs.profile
	if p.editLoaded {
		return
	}
	p.editLoaded = true
	p.editName = p.profile.Nickname
	p.editAvatar = p.profile.AvatarPath
}

// saveProfileEdit writes the nickname and the avatar path.
func (a *App) saveProfileEdit() {
	p := &a.prefs.profile
	name := strings.TrimSpace(p.editName)
	if name == "" {
		a.notice = "昵称不能为空"
		return
	}
	updated := p.profile
	updated.Nickname = name
	updated.AvatarPath = strings.TrimSpace(p.editAvatar)
	store := a.store
	if store == nil {
		p.profile = updated
		a.notice = "资料已保存（未连接数据库）"
		return
	}
	go func() {
		err := store.SaveProfile(context.Background(), updated)
		a.post(func() {
			if err != nil {
				a.notice = "保存失败：" + err.Error()
				return
			}
			p.profile = updated
			p.editLoaded = false
			a.notice = "资料已保存"
			a.router.Push("/profile")
		})
	}()
}

// pickAvatarPath asks for an image and records its path only.
func (a *App) pickAvatarPath() {
	if a.win == nil {
		a.notice = "选择头像只在桌面窗口里可用"
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:  a.win,
		Title:   "选择头像",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"jpg", "jpeg", "png", "webp"}}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.prefs.profile.editAvatar = paths[0]
	a.notice = "已记下头像路径，图片本身不复制"
}

// ensureCooked rebuilds the cooked page when the record history or a filter
// moved.
func (a *App) ensureCooked() {
	a.feed.ensure(a)
	c := &a.prefs.profile.cooked
	if c.applied && c.stamp == a.recordCount && c.stampLen == len(a.records) {
		return
	}
	a.applyCooked()
}

// applyCooked aggregates the record history into the cooked rows, applying
// the time, main-category and region filters and the chosen sort. It runs
// when the page opens or a filter changes, never while drawing.
func (a *App) applyCooked() {
	a.feed.ensure(a)
	c := &a.prefs.profile.cooked
	c.applied = true
	c.stamp = a.recordCount
	c.stampLen = len(a.records)
	now := time.Now()
	var cutoff time.Time
	switch c.timeFilter {
	case 1:
		cutoff = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	case 2:
		cutoff = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -90)
	}
	type aggregate struct {
		count int
		last  time.Time
	}
	aggs := make(map[int64]*aggregate)
	days := make(map[string]bool)
	records, seconds := 0, 0
	var months [12]int
	for _, r := range a.feed.records {
		at := r.CompletedAt.In(time.Local)
		if !cutoff.IsZero() && at.Before(cutoff) {
			continue
		}
		records++
		days[at.Format("2006-01-02")] = true
		months[int(at.Month())-1]++
		if r.SessionSeconds != nil {
			seconds += *r.SessionSeconds
		}
		agg, ok := aggs[r.RecipeID]
		if !ok {
			agg = &aggregate{}
			aggs[r.RecipeID] = agg
		}
		agg.count++
		if at.After(agg.last) {
			agg.last = at
		}
	}

	mains := make(map[string]bool)
	regions := make(map[string]bool)
	rows := make([]cookedRow, 0, len(aggs))
	for id, agg := range aggs {
		row := cookedRow{RecipeID: id, Name: a.recipeName(id), Count: agg.count, Last: agg.last}
		if rec, ok := a.recipeByID[id]; ok {
			row.Main = rec.MainCategory
			row.Region = rec.Region
			if rec.DurationMinutes != nil {
				row.Minutes = *rec.DurationMinutes
			}
		} else if meta, ok := a.feed.recipes[id]; ok {
			row.Main = meta.MainCategory
			row.Region = meta.Region
		}
		if row.Main != "" {
			mains[row.Main] = true
		}
		if row.Region != "" {
			regions[row.Region] = true
		}
		if c.main != "" && row.Main != c.main {
			continue
		}
		if c.region != "" && row.Region != c.region {
			continue
		}
		rows = append(rows, row)
	}
	applyCookedSort(rows, c.sortBy)
	c.rows = rows
	c.mains = sortedKeys(mains)
	c.regions = sortedKeys(regions)
	c.stats = cookedStats{
		Records: records, Recipes: len(rows), Days: len(days),
		Minutes: seconds / 60, Months: months,
	}
	top := append([]cookedRow{}, rows...)
	sort.Slice(top, func(i, j int) bool {
		if top[i].Count != top[j].Count {
			return top[i].Count > top[j].Count
		}
		return top[i].Name < top[j].Name
	})
	if len(top) > 5 {
		top = top[:5]
	}
	c.stats.Top = top
}

// applyCookedSort orders the rows in place.
func applyCookedSort(rows []cookedRow, mode int) {
	switch mode {
	case 1: // most cooked
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Count != rows[j].Count {
				return rows[i].Count > rows[j].Count
			}
			return rows[i].Name < rows[j].Name
		})
	case 2: // name
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	case 3: // longest first
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Minutes != rows[j].Minutes {
				return rows[i].Minutes > rows[j].Minutes
			}
			return rows[i].Name < rows[j].Name
		})
	default: // newest first
		sort.Slice(rows, func(i, j int) bool {
			if !rows[i].Last.Equal(rows[j].Last) {
				return rows[i].Last.After(rows[j].Last)
			}
			return rows[i].Name < rows[j].Name
		})
	}
}

// ensureShelf rebuilds the shelf list when the group, the counts or the
// search moved.
func (a *App) ensureShelf() {
	p := &a.prefs.profile
	s := &p.shelf
	sig := fmt.Sprintf("%d|%d|%d|%s", s.tab, len(p.profile.FavoriteRecipes),
		len(p.profile.WantedRecipes), s.query)
	if s.sig == sig {
		return
	}
	s.sig = sig
	a.applyShelf()
}

// applyShelf rebuilds the current shelf list and the search matches.
func (a *App) applyShelf() {
	p := &a.prefs.profile
	s := &p.shelf
	ids := p.profile.FavoriteRecipes
	if s.tab == 1 {
		ids = p.profile.WantedRecipes
	}
	known := make(map[int64]bool, len(ids))
	view := make([]shelfRow, 0, len(ids))
	for _, id := range ids {
		known[id] = true
		row := shelfRow{RecipeID: id, Name: a.recipeName(id)}
		if rec, ok := a.recipeByID[id]; ok {
			row.Sub = recipeSubtitle(rec)
		} else if meta, ok := a.feed.recipes[id]; ok {
			row.Sub = joinSub(meta.Cuisine, meta.Region)
		}
		view = append(view, row)
	}
	sort.Slice(view, func(i, j int) bool { return view[i].Name < view[j].Name })
	s.view = view

	query := strings.ToLower(strings.TrimSpace(s.query))
	found := make([]shelfRow, 0, 8)
	if query != "" {
		for _, rec := range a.recipes {
			if known[rec.ID] || len(found) >= 8 {
				continue
			}
			if !strings.Contains(strings.ToLower(rec.Name), query) {
				continue
			}
			found = append(found, shelfRow{RecipeID: rec.ID, Name: rec.Name, Sub: recipeSubtitle(rec)})
		}
	}
	s.found = found
}

// clearCooked empties the cooked history after the confirmation. The pages
// clear at once and the database catches up; a failed write reloads them.
func (a *App) clearCooked() {
	a.setRecords(nil, 0, 0)
	a.feed.clearRecords()
	a.applyCooked()
	store := a.store
	if store == nil {
		a.notice = "做过的菜已清空（未连接数据库）"
		return
	}
	go func() {
		err := store.ClearCookedRecords(context.Background())
		a.post(func() {
			if err != nil {
				a.notice = "清空失败：" + err.Error()
				a.reloadRecords()
				return
			}
			a.notice = "做过的菜已清空，收藏与等级保留"
		})
	}()
}

// recipeSubtitle is the one-line meta under a dish name.
func recipeSubtitle(rec domain.Recipe) string {
	return joinSub(rec.Cuisine, rec.Region)
}

func joinSub(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " · ")
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// profileEditPage edits the nickname and the avatar path.
func (a *App) profileEditPage(c *ui.Context, p design.Palette) {
	a.openProfileEdit()
	a.pageBody(c, func() {
		a.subHeader(c, p, "编辑资料", "昵称与头像只存在本机")
		box := design.CardPad(c, p)
		box.Children(func() {
			design.FieldLabel(c, p, "昵称")
			ui.TextInput(c, &a.prefs.profile.editName).Placeholder("怎么称呼你").
				Label("昵称输入").AutoFocus()
			design.FieldLabel(c, p, "头像路径")
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				ui.TextInput(c, &a.prefs.profile.editAvatar).Placeholder("桌面版只记录路径").
					Label("头像路径").Grow(1)
				if ui.Button(c, "选择图片").Clicked() {
					a.pickAvatarPath()
				}
			})
			ui.Text(c, "「我的」页默认头像留白；这里只在本机记下路径，不复制图片。").FontSize(11).TextColor(p.Faint)
		})
		ui.Row(c).Gap(10).Children(func() {
			if ui.PrimaryButton(c, "保存资料").Clicked() {
				a.saveProfileEdit()
			}
			if ui.Button(c, "取消").Clicked() {
				a.prefs.profile.editLoaded = false
				a.router.Push("/profile")
			}
		})
	})
}

// profileCookedPage lists the cooked dishes with filters, sorts and totals.
func (a *App) profileCookedPage(c *ui.Context, p design.Palette) {
	a.ensureCooked()
	co := &a.prefs.profile.cooked
	a.pageBody(c, func() {
		a.subHeader(c, p, "做过的菜",
			fmt.Sprintf("%d 条记录 · %d 道菜", co.stats.Records, co.stats.Recipes))

		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).
			Margin(0, 0, 12, 0).Children(func() {
			ui.Segmented(c, &co.view, "列表", "统计").Label("做过的菜的视图").Changed()
			if ui.Button(c, "清空记录").Clicked() {
				co.confirm = true
			}
		})

		// Filters: three single choices, so a segment and two menus, not a
		// wall of pills.
		ui.Row(c).Gap(design.Sp(c, 2)).AlignItems(ui.Center).Margin(0, 0, design.Sp(c, 2), 0).Wrap().Children(func() {
			if design.Segmented(c, &co.timeFilter, "全部时间", "本月", "近 90 天").Label("时间范围").Changed() {
				a.applyCooked()
			}
			if len(co.mains) > 1 {
				if selectFilter(c, &co.main, "主料不限", co.mains) {
					a.applyCooked()
				}
			}
			if len(co.regions) > 1 {
				if selectFilter(c, &co.region, "菜系不限", co.regions) {
					a.applyCooked()
				}
			}
		})

		if co.view == 1 {
			a.cookedStatsBody(c, p)
			return
		}

		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Margin(0, 0, 12, 0).Children(func() {
			ui.Text(c, "排序").FontSize(11).TextColor(p.Faint)
			if ui.Segmented(c, &co.sortBy, "最近", "最常做", "菜名", "耗时").
				Label("排序方式").Changed() {
				a.applyCooked()
			}
		})

		if len(co.rows) == 0 {
			box := design.EmptyState(c, p, &icons.Bowl, "这个筛选下没有做过的菜",
				"换一个时间范围，或者去掌序做一道菜。")
			box.Children(func() {
				if ui.Button(c, "去掌序").Clicked() {
					a.router.Push("/schedule")
				}
			})
			return
		}
		ui.Column(c).FillWidth().Gap(1).Radius(14).Background(p.Surface).
			Border(1, p.Border).Children(func() {
			for _, row := range co.rows {
				row := row
				ui.Row(c).FillWidth().Padding(12, 10).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, row.Name).FontSize(13).FontWeight(600).TextColor(p.Text).SingleLine()
						sub := joinSub(row.Main, row.Region)
						if !row.Last.IsZero() {
							sub = joinSub(sub, "最近 "+row.Last.In(time.Local).Format("01-02"))
						}
						ui.Text(c, sub).FontSize(11).TextColor(p.Muted).SingleLine()
					})
					if row.Minutes > 0 {
						ui.Text(c, fmt.Sprintf("%.0f 分钟", row.Minutes)).FontSize(11).TextColor(p.Faint)
					}
					ui.Text(c, fmt.Sprintf("%d 次", row.Count)).FontSize(11).TextColor(p.Accent)
				})
			}
		})

		if ui.AlertDialog(c, &co.confirm, "清空做过的菜？",
			"记录与统计会归零，收藏和等级保留。这个操作不能撤销。", "取消", "清空") == 1 {
			a.clearCooked()
		}
	})
}

// cookedStatsBody is the totals view of the cooked page.
func (a *App) cookedStatsBody(c *ui.Context, p design.Palette) {
	s := a.prefs.profile.cooked.stats
	ui.Row(c).FillWidth().Gap(design.Sp(c, 4)).Wrap().Margin(0, 0, 16, 0).Children(func() {
		design.Stat(c, p, fmt.Sprint(s.Records), "条记录")
		design.Stat(c, p, fmt.Sprint(s.Recipes), "做过菜式")
		design.Stat(c, p, fmt.Sprint(s.Days), "开饭天数")
		design.Stat(c, p, fmt.Sprint(s.Minutes), "总分钟")
	})
	a.section(c, p, "月份分布", func() {
		box := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
		box.Children(func() {
			labels := make([]string, 12)
			values := make([]float64, 12)
			for i := 0; i < 12; i++ {
				labels[i] = fmt.Sprint(i + 1)
				values[i] = float64(s.Months[i])
			}
			design.BarChart(c, p, labels, values, 140)
			ui.Text(c, "按完成月份统计的记录条数。").FontSize(11).TextColor(p.Faint)
		})
	})
	a.section(c, p, "做得最多", func() {
		if len(s.Top) == 0 {
			ui.Text(c, "这个筛选下没有记录。").FontSize(12).TextColor(p.Muted)
			return
		}
		box := ui.Column(c).FillWidth().Gap(design.Sp(c, 1))
		box.Children(func() {
			top := s.Top[0].Count
			for _, row := range s.Top {
				row := row
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, row.Name).FontSize(12).TextColor(p.Text).Grow(1).SingleLine()
					ui.Text(c, fmt.Sprintf("%d 次", row.Count)).FontSize(11).TextColor(p.Muted)
				})
				ui.Meter(c, float64(row.Count)/float64(top), 0, 1, nil).Label(row.Name + " 的次数占比")
			}
		})
	})
}

// profileShelfPage is 我的收藏 / 想要做.
func (a *App) profileShelfPage(c *ui.Context, p design.Palette) {
	pf := &a.prefs.profile
	a.syncFavorites()
	a.ensureShelf()
	a.pageBody(c, func() {
		a.subHeader(c, p, "我的菜谱", fmt.Sprintf("%d 个收藏 · %d 个想要",
			len(pf.profile.FavoriteRecipes), len(pf.profile.WantedRecipes)))

		if design.Segmented(c, &pf.shelf.tab, "收藏", "想要做").Label("菜谱分组").Changed() {
			a.applyShelf()
		}

		ui.Row(c).FillWidth().Gap(10).Children(func() {
			if ui.SearchField(c, &pf.shelf.query).Placeholder("搜一道菜，加到当前分组").
				Label("搜索菜式").Grow(1).Changed() {
				a.applyShelf()
			}
			if ui.Button(c, "去菜式页").Clicked() {
				a.router.Push("/recipes")
			}
		})
		if pf.shelf.query != "" {
			if len(pf.shelf.found) == 0 {
				ui.Text(c, "没有找到这道菜。").FontSize(12).TextColor(p.Muted)
			}
			ui.Column(c).FillWidth().Gap(1).Radius(14).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				for _, row := range pf.shelf.found {
					row := row
					ui.Row(c).FillWidth().Padding(12, 10).Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, row.Name).FontSize(13).TextColor(p.Text).SingleLine()
							if row.Sub != "" {
								ui.Text(c, row.Sub).FontSize(11).TextColor(p.Muted).SingleLine()
							}
						})
						if ui.Button(c, "加入").Clicked() {
							a.addShelf(row.RecipeID)
						}
					})
				}
			})
		}

		ids := pf.profile.FavoriteRecipes
		if pf.shelf.tab == 1 {
			ids = pf.profile.WantedRecipes
		}
		if len(ids) == 0 {
			title, body := "还没有收藏", "在菜式详情或这里搜索，把想反复做的菜收进来。"
			if pf.shelf.tab == 1 {
				title, body = "还没有想要的菜", "搜索一道菜加入，下次配餐和掌序会优先想到它。"
			}
			box := design.EmptyState(c, p, &icons.Star, title, body)
			box.Children(func() {
				if ui.PrimaryButton(c, "去菜式页挑一道").Clicked() {
					a.router.Push("/recipes")
				}
			})
			return
		}

		ui.Column(c).FillWidth().Gap(1).Radius(14).Background(p.Surface).
			Border(1, p.Border).Children(func() {
			for _, row := range pf.shelf.view {
				row := row
				ui.Row(c).FillWidth().Padding(12, 10).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, row.Name).FontSize(13).FontWeight(600).TextColor(p.Text).SingleLine()
						if row.Sub != "" {
							ui.Text(c, row.Sub).FontSize(11).TextColor(p.Muted).SingleLine()
						}
					})
					if ui.Button(c, "去掉").Clicked() {
						a.removeShelf(row.RecipeID)
					}
				})
			}
		})
	})
}

// addShelf puts a dish into the group on screen.
func (a *App) addShelf(id int64) {
	if a.prefs.profile.shelf.tab == 1 {
		a.persistWanted(id, true)
	} else {
		a.persistFavorite(id, true)
	}
	a.prefs.profile.shelf.query = ""
	a.applyShelf()
}

// removeShelf takes a dish out of the group on screen.
func (a *App) removeShelf(id int64) {
	if a.prefs.profile.shelf.tab == 1 {
		a.persistWanted(id, false)
	} else {
		a.persistFavorite(id, false)
	}
	a.applyShelf()
}
