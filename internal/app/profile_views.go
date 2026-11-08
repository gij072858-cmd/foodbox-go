package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
)

// profileEntry is one row of a profile group. label is the accessible name,
// which the tests address because several page labels repeat a statistic.
type profileEntry struct {
	title, sub, label string
	onClick           func()
}

// profilePage is 我的: identity, the numbers, the two entry groups and the
// latest cooking. Everything read here was loaded at bootstrap or after a
// write; the page itself only draws.
func (a *App) profilePage(c *ui.Context, p design.Palette) {
	pf := &a.prefs.profile
	a.syncFavorites()
	a.pageBody(c, func() {
		a.profileHeader(c, p)
		a.profileStats(c, p)

		ui.Row(c).FillWidth().Gap(16).Margin(0, 0, 24, 0).AlignItems(ui.Start).Children(func() {
			a.profileGroup(c, p, "我的菜谱", []profileEntry{
				{title: "收藏", label: "打开收藏",
					sub: fmt.Sprintf("%d 道想反复做的菜", len(pf.profile.FavoriteRecipes)),
					onClick: func() {
						pf.shelf.tab = 0
						pf.shelf.sig = ""
						a.router.Push("/profile/shelf")
					}},
				{title: "做过的菜", label: "打开做过的菜",
					sub:     fmt.Sprintf("%d 条记录", a.recordCount),
					onClick: func() { a.router.Push("/profile/cooked") }},
				{title: "想要的菜", label: "打开想要的菜",
					sub: fmt.Sprintf("%d 道还没做", len(pf.profile.WantedRecipes)),
					onClick: func() {
						pf.shelf.tab = 1
						pf.shelf.sig = ""
						a.router.Push("/profile/shelf")
					}},
			})
			a.profileGroup(c, p, "厨房", []profileEntry{
				{title: "身体档案", label: "打开身体档案", sub: "身高体重、目标与口味约束",
					onClick: func() { a.router.Push("/sichu/body") }},
				{title: "同桌成员", label: "打开同桌成员", sub: "一起吃饭的人与忌口",
					onClick: func() { a.router.Push("/sichu/members") }},
				{title: "设置", label: "打开设置", sub: "外观动效、数据备份与关于",
					onClick: func() { a.router.Push("/settings") }},
				{title: "帮助", label: "打开帮助", sub: "分组说明与快捷键",
					onClick: func() { a.router.Push("/help") }},
			})
		})

		a.profileRecent(c, p)

		if len(pf.profile.FavoriteRecipes) == 0 && a.recordCount == 0 {
			box := design.EmptyState(c, p, &icons.Profile, "还没有留下痕迹",
				"做完第一道菜、收藏一道菜之后，这里会记下你的口味与习惯。")
			box.Children(func() {
				ui.Row(c).Gap(10).Children(func() {
					if ui.PrimaryButton(c, "去掌序做一道菜").Clicked() {
						a.router.Push("/schedule")
					}
					if ui.Button(c, "去菜式挑一道").Clicked() {
						a.router.Push("/recipes")
					}
				})
			})
		}
	})
}

// profileHeader is the identity card: avatar initial, nickname and level.
func (a *App) profileHeader(c *ui.Context, p design.Palette) {
	pf := &a.prefs.profile
	nickname := pf.profile.Nickname
	if nickname == "" {
		nickname = "食匣用户"
	}
	level := XPLevel(pf.profile.XP)

	ui.Row(c).FillWidth().Gap(16).Padding(20).Radius(18).Background(p.Surface).
		Border(1, p.Border).Margin(0, 0, 20, 0).AlignItems(ui.Center).Children(func() {
		// The default face is blank on purpose: a generated initial is a
		// placeholder wearing the user's name. A recorded avatar path is not
		// copied or rendered, so the circle stays a quiet, empty well.
		ui.Box(c).Size(64, 64).Radius(32).Background(p.SurfaceAlt).
			Border(1, p.Border).Role(ui.RoleImage).Label("头像")
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Justify(ui.Center).Children(func() {
			ui.Text(c, nickname).FontSize(20).FontWeight(700).TextColor(p.Text).SingleLine()
			ui.Text(c, fmt.Sprintf("Lv.%d", level)).FontSize(12).TextColor(p.Muted)
			ui.Progress(c, float64(XPProgress(pf.profile.XP))).Label("等级进度")
		})
		if ui.Button(c, "编辑资料").Clicked() {
			a.router.Push("/profile/edit")
		}
	})
}

// profileStats is the four-number strip.
func (a *App) profileStats(c *ui.Context, p design.Palette) {
	pf := &a.prefs.profile
	ui.Row(c).FillWidth().Gap(design.Sp(c, 4)).Wrap().Margin(0, 0, 24, 0).Children(func() {
		design.Stat(c, p, fmt.Sprint(a.recordCount), "做过的菜")
		design.Stat(c, p, fmt.Sprint(len(pf.profile.FavoriteRecipes)), "收藏")
		design.Stat(c, p, fmt.Sprint(len(pf.profile.WantedRecipes)), "想要做")
		design.Stat(c, p, fmt.Sprint(a.mealDays), "开饭天数")
	})
}

// profileGroup draws one titled card of entry rows.
func (a *App) profileGroup(c *ui.Context, p design.Palette, title string, entries []profileEntry) {
	ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
		ui.Text(c, title).FontSize(15).FontWeight(700).TextColor(p.Text).Role(ui.RoleHeading)
		box := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
		box.Children(func() {
			for i, entry := range entries {
				entry := entry
				if i > 0 {
					ui.Divider(c)
				}
				label := entry.label
				if label == "" {
					label = entry.title
				}
				row := design.TileInCard(c, p, "profile-entry-"+entry.title,
					entry.title, entry.sub, func() {
						ui.Icon(c, &icons.Chevron).Size(14, 14).TextColor(p.Ink)
					}).Height(54).Label(label)
				if row.Clicked() && entry.onClick != nil {
					entry.onClick()
				}
			}
		})
	})
}

// profileRecent is the latest cooking summary.
func (a *App) profileRecent(c *ui.Context, p design.Palette) {
	a.section(c, p, "最近下厨", func() {
		if len(a.records) == 0 {
			ui.Text(c, "还没有开饭记录，做完第一道菜就会出现在这里。").FontSize(12).TextColor(p.Muted)
			if ui.Button(c, "去掌序").Clicked() {
				a.router.Push("/schedule")
			}
			return
		}
		shown := 0
		box := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
		box.Children(func() {
			for _, r := range a.records {
				if shown >= 3 {
					break
				}
				shown++
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, r.CompletedAt.In(time.Local).Format("01-02 15:04")).
						FontSize(11).TextColor(p.Faint).Width(84)
					ui.Text(c, a.recipeName(r.RecipeID)).FontSize(13).TextColor(p.Text).Grow(1).SingleLine()
					if d := recordDuration(r); d != "" {
						ui.Text(c, d).FontSize(11).TextColor(p.Muted)
					}
				})
			}
			ui.Row(c).Margin(2, 0, 0, 0).Children(func() {
				if ui.Button(c, "看全部做过的菜").Clicked() {
					a.router.Push("/profile/cooked")
				}
			})
		})
	})
}

// firstRune is defined with the recipe pages: the avatar reuses it.
