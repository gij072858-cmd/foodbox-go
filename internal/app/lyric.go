package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
)

var demoLyric = []string{
	"先把米淘好，按下煮饭键",
	"肉切细丝，加生抽与淀粉抓匀",
	"青椒去籽切丝，蒜拍碎",
	"热锅冷油，下肉丝滑散",
	"肉色变白就盛出，别炒老",
	"底油下蒜末与青椒，大火快炒",
	"青椒断生，回锅肉丝",
	"沿锅边淋一勺生抽",
	"加盐、少许糖，翻匀",
	"关火前撒一点白胡椒",
	"装盘，趁热吃",
	"锅还热，顺手把灶台擦一下",
}

const lyricRowHeight float32 = 92

// lyricOptions shape one flow. Anchor is where the lit step sits in the
// viewport (0.5 is dead centre, 0.42 is Cloudy's slightly-high focus); Lit
// paints the current row's pale band, Head and Tail build only beside or
// above that row, and Pick makes every line a way into its step.
type lyricOptions struct {
	Anchor float32
	Lit    bool
	Head   func(i int)
	Tail   func(i int)
	Pick   func(i int)
}

// lyricFlow is the schedule page's lyric-style step flow: the step nearest
// the anchor is lit and slightly larger. The offset is computed from the
// scroll state each frame, so scrolling is the animation; the springs only
// smooth the change of focus.
func (a *App) lyricFlow(c *ui.Context, p design.Palette, height float32, lines []string, opt lyricOptions) {
	if opt.Anchor <= 0 || opt.Anchor >= 1 {
		opt.Anchor = 0.5
	}
	if a.level == design.MotionOff {
		a.lyricFlowStill(c, p, height, lines, opt)
		return
	}
	lead, trail := lyricSpacers(height, opt.Anchor)
	center := a.lyric.Y + height*opt.Anchor - lead
	scroll := ui.Scroll(c.Key("lyric-flow")).Fill().TrackScroll(&a.lyric)
	scroll.Children(func() {
		ui.Column(c).FillWidth().Padding(0, 28).Children(func() {
			// Leading and trailing room let the first and last steps reach
			// the focus point, not just the middle ones. The two ends differ
			// when the focus is not dead centre, so the last row can still
			// reach the anchor.
			ui.Box(c).Height(lead)
			for i, line := range lines {
				i, line := i, line
				pos := float32(i)*lyricRowHeight + lyricRowHeight/2
				d := absF(pos-center) / lyricRowHeight
				current := d < 0.5
				opacity, size, scale := float32(0.35), float32(18), float32(0.98)
				if current {
					opacity, size, scale = 1, design.FontHero, 1.02
				}
				op := scroll.AnimateTo(fmt.Sprintf("lyric-op-%d", i), opacity, ui.Snappy)
				sc := scroll.AnimateTo(fmt.Sprintf("lyric-scale-%d", i), scale, ui.Snappy)
				row := ui.Row(c.Key(fmt.Sprintf("lyric-row-%d", i))).FillWidth().Height(lyricRowHeight).
					AlignItems(ui.Center).Radius(12).Role(ui.RoleListItem)
				if current && opt.Lit {
					row.Background(p.AccentPale.Alpha(0.55))
				}
				row.Children(func() {
					body := ui.ButtonBase(c.Key(fmt.Sprintf("lyric-body-%d", i))).
						Column().Grow(1).MinWidth(0).AlignItems(ui.Start).Gap(2).Label(line)
					body.Children(func() {
						if current && opt.Head != nil {
							opt.Head(i)
						}
						ui.Text(c, line).FontSize(size).FontWeight(700).TextColor(p.Text).
							Opacity(op).Scale(sc, sc).LineHeight(1.25).Grow(1).MinWidth(0)
					})
					if body.Clicked() && opt.Pick != nil {
						opt.Pick(i)
					}
					if current && opt.Tail != nil {
						opt.Tail(i)
					}
				})
			}
			ui.Box(c).Height(trail)
		})
	})
}

func (a *App) lyricFlowStill(c *ui.Context, p design.Palette, height float32, lines []string, opt lyricOptions) {
	lead, trail := lyricSpacers(height, opt.Anchor)
	center := a.lyric.Y + height*opt.Anchor - lead
	scroll := ui.Scroll(c.Key("lyric-flow")).Fill().TrackScroll(&a.lyric)
	scroll.Children(func() {
		ui.Column(c).FillWidth().Padding(0, 28).Children(func() {
			ui.Box(c).Height(lead)
			for i, line := range lines {
				i, line := i, line
				pos := float32(i)*lyricRowHeight + lyricRowHeight/2
				d := absF(pos-center) / lyricRowHeight
				current := d < 0.5
				opacity, size, scale := float32(0.35), float32(18), float32(0.98)
				if current {
					opacity, size, scale = 1, design.FontHero, 1.02
				}
				row := ui.Row(c.Key(fmt.Sprintf("lyric-row-%d", i))).FillWidth().Height(lyricRowHeight).
					AlignItems(ui.Center).Radius(12).Role(ui.RoleListItem)
				if current && opt.Lit {
					row.Background(p.AccentPale.Alpha(0.55))
				}
				row.Children(func() {
					body := ui.ButtonBase(c.Key(fmt.Sprintf("lyric-body-%d", i))).
						Column().Grow(1).MinWidth(0).AlignItems(ui.Start).Gap(2).Label(line)
					body.Children(func() {
						if current && opt.Head != nil {
							opt.Head(i)
						}
						ui.Text(c, line).FontSize(size).FontWeight(700).TextColor(p.Text).
							Opacity(opacity).Scale(scale, scale).LineHeight(1.25).Grow(1).MinWidth(0)
					})
					if body.Clicked() && opt.Pick != nil {
						opt.Pick(i)
					}
					if current && opt.Tail != nil {
						opt.Tail(i)
					}
				})
			}
			ui.Box(c).Height(trail)
		})
	})
}

// hangHandle is the current wait step's way into the tray: a compact pill
// beside the line, still draggable. Moving the pointer a few DIPs while it
// is pressed lifts the step and carries a copy of it (the framework's Drag,
// the same thing Flutter's Draggable feedback is); letting go over the band
// hangs the countdown. A plain tap hangs it too.
func (a *App) hangHandle(c *ui.Context, p design.Palette, line int) {
	handle := ui.ButtonBase(c.Key(fmt.Sprintf("hang-handle-%d", line))).Row().
		Height(28).Shrink(0).Padding(2, 9).Gap(5).Radius(design.RadiusPill).
		AlignItems(ui.Center).Label(fmt.Sprintf("把第 %d 步挂起来计时", line+1))
	design.Wash(handle, p.SurfaceAlt, p.Hover)
	handle.Children(func() {
		ui.Icon(c, &icons.Clock).Size(13, 13).TextColor(p.Ink)
		ui.Text(c, "挂起").FontSize(design.FontCaption).FontWeight(600).TextColor(p.Text)
	})
	handle.Drag(hangDrag{line: line})
	if handle.Dragging() {
		a.sched.dragging = true
		handle.Opacity(0.4)
	}
	if handle.Clicked() {
		a.hangLine(line)
	}
}

func absF(v float32) float32 { return float32(math.Abs(float64(v))) }

// focusLyric puts step i in the middle of the flow. The offset is set in
// one frame; the rows' springs do the visual catching up, so no ScrollTo
// asks for a rebuild on every frame of the glide. lines may be nil when only
// the offset matters (restoring a run before its queue snapshot is loaded).
func (a *App) focusLyric(i int, lines []string) {
	if i < 0 {
		i = 0
	}
	if len(lines) > 0 && i >= len(lines) {
		i = len(lines) - 1
	}
	a.lyricFocus = i
	a.lyric.Y = float32(i) * lyricRowHeight
}

// glideLyric moves the flow to step i as a scroll, not a jump. Only the full
// motion level glides; Lite and Off land in one frame, and the framework
// still settles at once when the system asks for less motion.
func (a *App) glideLyric(i int) {
	if i < 0 {
		i = 0
	}
	if last := len(a.sched.runLines) - 1; last >= 0 && i > last {
		i = last
	}
	a.lyricFocus = i
	offset := float32(i) * lyricRowHeight
	if a.level.Full() {
		a.lyric.ScrollTo(0, offset, design.Pane)
		return
	}
	a.lyric.Y = offset
}

// lyricSpacers is the room above and below the rows that lets the first and
// last rows reach the focus point. They differ when the anchor is not dead
// centre: with the same value at both ends the scroll runs out early and the
// last row can never become current, which hid its controls.
func lyricSpacers(height, anchor float32) (lead, trail float32) {
	lead = height*anchor - lyricRowHeight/2
	trail = height*(1-anchor) - lyricRowHeight/2
	if lead < 0 {
		lead = 0
	}
	if trail < 0 {
		trail = 0
	}
	return lead, trail
}

// lyricCenterOffset puts row i's middle at the viewport's focus point in one
// frame. With the spacer above, the first row centers at offset 0.
func lyricCenterOffset(i int, _ float32) float32 {
	if i < 0 {
		i = 0
	}
	return float32(i) * lyricRowHeight
}

// schedulePage is the queue: what is waiting to be cooked, what the parallel
// plan saves, and the way into prep, timeline, run and settlement.
func (a *App) schedulePage(c *ui.Context, p design.Palette) {
	count, minutes := a.scheduleTotals()
	a.pageBody(c, func() {
		ui.Row(c).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).Margin(0, 0, 16, 0).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, &icons.Schedule).Size(20, 20).TextColor(p.Ink)
				ui.Text(c, "待做队列").FontSize(18).FontWeight(700).TextColor(p.Text)
				design.Pill(c, p, fmt.Sprintf("%d 道", count))
				design.Pill(c, p, fmt.Sprintf("%d 样待备料", len(a.sched.prep)))
			})
			ui.Text(c, fmt.Sprintf("%d 道菜 · 预计 %d 分钟", count, minutes)).
				FontSize(12).TextColor(p.Muted)
		})

		if count == 0 {
			// The shared empty panel, so a big "nothing here" card wears the
			// same faint theme tint as the pantry's.
			empty := design.EmptyState(c, p, &icons.Schedule, "掌序里还没有菜",
				"在菜式页挑一道，点「加入掌序」。这里会把多道菜的步骤排成一条时间线。")
			empty.Children(func() {
				if ui.PrimaryButton(c, "去挑一道菜").Clicked() {
					a.router.Push("/recipes")
				}
			})
			return
		}

		// The plan's numbers: wall-clock length, the serial sum it saves from,
		// and how many ingredients the prep list aggregates.
		ui.Row(c).FillWidth().Gap(design.Sp(c, 4)).Wrap().Children(func() {
			design.Stat(c, p, fmt.Sprintf("%d 分钟", a.sched.timeline.TotalMinutes), "并行后总时长")
			design.Stat(c, p, fmt.Sprintf("%d 分钟", a.sched.timeline.SerialMinutes), "一步一步做")
			design.Stat(c, p, fmt.Sprintf("%d 分钟", a.sched.timeline.SavedMinutes), "并行省下")
			design.Stat(c, p, fmt.Sprintf("%d 样", len(a.sched.prep)), "备料")
		})

		if a.sched.runActive {
			a.runResumeCard(c, p)
		}

		groups := a.scheduleMealGroups()
		a.section(c, p, "待做", func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 2)).Children(func() {
				for _, group := range groups {
					group := group
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).
						Margin(0, 0, design.Sp(c, 1), 0).Children(func() {
						ui.Text(c, mealGroupLabel(group.Meal)).FontSize(design.FontSubhead).
							FontWeight(600).TextColor(p.Text)
						design.Pill(c, p, fmt.Sprintf("%d 道", len(group.Items)))
						ui.Box(c).Grow(1)
						if a.sched.runActive {
							return
						}
						short := mealGroupLabel(group.Meal)
						startLabel := "开始掌序"
						settleLabel := "结算并扣库存"
						if a.pantryOff {
							settleLabel = "结算这桌"
						}
						if len(groups) > 1 {
							startLabel = "开始" + short
							settleLabel = "结算" + short
						}
						if ui.Button(c, settleLabel).Clicked() {
							a.openSettleConfirmFor(group.Meal)
						}
						if ui.PrimaryButton(c, startLabel).Clicked() {
							a.startRunMeal(group.Meal)
							if a.sched.runActive {
								a.router.Push("/schedule/run")
							}
						}
					})
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						for _, item := range group.Items {
							item := item
							a.queueRow(c, p, item)
						}
					})
				}
			})
		})

		a.section(c, p, "接下来", func() {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "备料清单").Clicked() {
					a.router.Push("/schedule/prep")
				}
				if ui.Button(c, "排程时间线").Clicked() {
					a.router.Push("/schedule/timeline")
				}
				ui.Box(c).Grow(1)
				if a.sched.runActive {
					if ui.PrimaryButton(c, "继续掌序").Clicked() {
						a.startOrResumeRun()
					}
				}
			})
		})

	})
	a.settleSheet(c, p)
}

// runResumeCard offers to continue a restored run.
func (a *App) runResumeCard(c *ui.Context, p design.Palette) {
	total := len(a.sched.runLines)
	step := a.sched.runStep + 1
	elapsed := a.sched.elapsed(time.Now().Unix())
	ui.Row(c).FillWidth().Padding(14, 12).Gap(12).AlignItems(ui.Center).Radius(14).
		Background(p.AccentPale).Margin(0, 0, 16, 0).Children(func() {
		ui.Icon(c, &icons.Clock).Size(18, 18).TextColor(p.Ink)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, fmt.Sprintf("上次做到第 %d / %d 步", step, total)).
				FontSize(13).FontWeight(600).TextColor(p.Text)
			ui.Text(c, fmt.Sprintf("已用 %s，接着往下做就行", clockText(elapsed))).
				FontSize(11).TextColor(p.Muted)
		})
		if ui.Button(c, "放弃").Clicked() {
			a.abandonRun()
		}
		if ui.PrimaryButton(c, "继续掌序").Clicked() {
			a.router.Push("/schedule/run")
		}
	})
}

// queueRow is one dish in the待做 queue.
func (a *App) queueRow(c *ui.Context, p design.Palette, item domain.ScheduleItem) {
	rec, ok := a.recipeByID[item.RecipeID]
	if !ok {
		return
	}
	steps := len(a.steps[rec.ID])
	ui.Row(c).FillWidth().Padding(14, 12).Gap(14).AlignItems(ui.Center).Radius(12).
		Background(p.Surface).Border(1, p.Border).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, rec.Name).FontSize(14).FontWeight(600).TextColor(p.Text).SingleLine()
			meta := fmt.Sprintf("%d 人份 · %d 步", max(item.Servings, 1), steps)
			if duration := rec.DurationText(); duration != "" {
				meta += " · " + duration
			}
			ui.Text(c, meta).FontSize(11).TextColor(p.Muted).SingleLine()
		})
		if res := a.recipeMatches[rec.ID]; !a.pantryOff && res.Status.Label() != "" {
			color := p.Faint
			switch res.Status {
			case cookmatch.CanCook:
				color = p.Success
			case cookmatch.Almost:
				color = p.Warning
			}
			ui.Text(c, res.Status.Label()).FontSize(11).FontWeight(600).TextColor(color)
		}
		if ui.Button(c, "移出").Clicked() {
			a.removeSchedule(rec.ID)
		}
	})
}

// clockText is "mm:ss" for a duration in seconds.
func clockText(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}
