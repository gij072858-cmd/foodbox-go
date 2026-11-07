package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// scheduleTabs are the M4 sub-pages the sub-navigation offers.
var scheduleTabs = []struct {
	label string
	path  string
}{
	{"队列", "/schedule"},
	{"备料", "/schedule/prep"},
	{"时间线", "/schedule/timeline"},
	{"执行", "/schedule/run"},
}

// scheduleSubnav is the back link plus the four M4 pages' tabs, shared by all
// of them so the section reads as one place.
func (a *App) scheduleSubnav(c *ui.Context, p design.Palette, active string) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Margin(0, 0, 18, 0).Children(func() {
		ui.BackButton(c, a.router)
		ui.Text(c, "掌序").FontSize(12).TextColor(p.Muted)
		labels := make([]string, len(scheduleTabs))
		idx := 0
		for i, tab := range scheduleTabs {
			labels[i] = tab.label
			if tab.path == active {
				idx = i
			}
		}
		if ui.Tabs(c, &idx, labels...).Changed() {
			a.router.Push(scheduleTabs[idx].path)
		}
	})
}

// preparePage is the aggregated prep list of the queued dishes: one row per
// ingredient, in three blocks (fresh, staples, seasonings).
func (a *App) schedulePrepPage(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		a.scheduleSubnav(c, p, "/schedule/prep")
		missing, covered := prepGapCounts(a.sched.prep)
		ui.Row(c).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).Margin(0, 0, 16, 0).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "总备料清单").FontSize(18).FontWeight(700).TextColor(p.Text)
				line := fmt.Sprintf("这桌 %d 样食材 · 在库 %d 样 · 缺口 %d 样",
					len(a.sched.prep), covered, missing)
				if a.pantryOff {
					line = fmt.Sprintf("这桌 %d 样食材", len(a.sched.prep))
				}
				ui.Text(c, line).FontSize(12).TextColor(p.Muted)
			})
			if !a.pantryOff {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if missing == 0 {
						ui.Text(c, "料齐了").FontSize(12).FontWeight(600).TextColor(p.Success)
					} else {
						ui.Text(c, fmt.Sprintf("还差 %d 样", missing)).FontSize(12).FontWeight(600).TextColor(p.Warning)
					}
				})
			}
		})

		if len(a.sched.prep) == 0 {
			empty := design.EmptyState(c, p, nil, "这桌还没有要备的料",
				"先往掌序里加几道菜，再回来汇总备料单。")
			empty.Children(func() {
				if ui.PrimaryButton(c, "去挑一道菜").Clicked() {
					a.router.Push("/recipes")
				}
			})
			return
		}

		hasRawMeat := false
		for _, line := range a.sched.prep {
			if line.RawMeat {
				hasRawMeat = true
				break
			}
		}
		if hasRawMeat {
			ui.Row(c).FillWidth().Padding(12, 10).Gap(10).AlignItems(ui.Center).Radius(12).
				Background(p.Warning.Alpha(0.1)).Margin(0, 0, 14, 0).Children(func() {
				ui.Icon(c, &icons.Alert).Size(15, 15).TextColor(p.Ink)
				ui.Text(c, "这桌有生肉：先切素的，肉切完把案板和刀冲一遍再碰即食的").
					FontSize(12).TextColor(p.Text)
			})
		}

		for _, group := range a.sched.prepGroups {
			group := group
			a.section(c, p, fmt.Sprintf("%s · %d 样", group.Title, len(group.Lines)), func() {
				ui.Column(c).FillWidth().Radius(14).Background(p.Surface).
					Border(1, p.Border).Clip().Children(func() {
					for i, line := range group.Lines {
						line := line
						if i > 0 {
							ui.Box(c).FillWidth().Height(1).Background(p.Border)
						}
						a.prepRow(c, p, line)
					}
				})
			})
		}

		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			if !a.pantryOff && ui.PrimaryButton(c, "缺口加入购物清单").Clicked() {
				a.addPrepGapsToShopping()
			}
			startLabel := "开始掌序"
			if a.sched.runActive {
				startLabel = "继续掌序"
			}
			if ui.Button(c, startLabel).Clicked() {
				a.startOrResumeRun()
			}
			if !a.pantryOff {
				ui.Text(c, "颗粒度在库算有；调料只列不减。").FontSize(11).TextColor(p.Faint)
			}
		})
	})
}

// prepRow is one aggregated ingredient line.
func (a *App) prepRow(c *ui.Context, p design.Palette, line domain.PrepLine) {
	ui.Row(c).FillWidth().Padding(14, 11).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, line.Name).FontSize(13).FontWeight(600).TextColor(p.Text).SingleLine()
				if line.RawMeat {
					design.Tag(c, p, "生", p.Danger)
				}
			})
			if len(line.Recipes) > 0 {
				ui.Text(c, strings.Join(line.Recipes, "、")).FontSize(11).
					TextColor(p.Muted).SingleLine()
			}
		})
		ui.Text(c, prepAmountText(line)).FontSize(12).FontWeight(600).
			TextColor(p.Text).SingleLine()
		if !a.pantryOff {
			ui.Text(c, prepStockText(line)).FontSize(11).
				TextColor(prepStockColor(p, line)).Width(66).Shrink(0).
				TextAlign(ui.End).SingleLine()
		}
	})
}

// prepAmountText is the aggregated needed amount, or 适量 when no amount was
// written.
func prepAmountText(line domain.PrepLine) string {
	if line.Needed <= 0 {
		return "适量"
	}
	return domain.AmountText(line.Needed, line.Unit)
}

// prepStockText says where the ingredient stands.
func prepStockText(line domain.PrepLine) string {
	switch {
	case line.Seasoning:
		return "按口味"
	case !line.InStock:
		if line.Needed > 0 {
			return "缺 " + domain.AmountText(line.Needed, line.Unit)
		}
		return "缺"
	case line.Missing > 0:
		return "缺 " + domain.AmountText(line.Missing, line.Unit)
	case line.Have > 0:
		return "在库 " + domain.AmountText(line.Have, line.Unit)
	default:
		return "在库"
	}
}

func prepStockColor(p design.Palette, line domain.PrepLine) ui.Color {
	switch {
	case line.Seasoning:
		return p.Faint
	case !line.InStock || line.Missing > 0:
		return p.Warning
	default:
		return p.Success
	}
}

// prepGapCounts counts the lines that still have to be bought and the ones
// already covered.
func prepGapCounts(lines []domain.PrepLine) (missing, covered int) {
	for _, line := range lines {
		if line.Seasoning {
			continue
		}
		if line.InStock && line.Missing <= 0 {
			covered++
		} else {
			missing++
		}
	}
	return missing, covered
}

// scheduleTimelinePage draws the parallel plan: one row per resource slot,
// each step a block placed by its minutes. The stove count is the kitchen's
// and re-plans live.
func (a *App) scheduleTimelinePage(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		a.scheduleSubnav(c, p, "/schedule/timeline")
		tl := a.sched.timeline
		ui.Row(c).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).Margin(0, 0, 16, 0).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "排程时间线").FontSize(18).FontWeight(700).TextColor(p.Text)
				ui.Text(c, "同一条轨道上不会同时做两件事；灶台数决定能同时开几口锅").
					FontSize(12).TextColor(p.Muted)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "灶台").FontSize(11).TextColor(p.Muted)
				if ui.Segmented(c, &a.sched.stoveChoice, "1 灶", "2 灶", "3 灶", "4 灶").Changed() {
					a.setStoveCount(a.sched.stoveChoice)
				}
			})
		})

		if tl.Empty() {
			empty := design.EmptyState(c, p, nil, "还没有排得上的步骤",
				"掌序里的菜需要先有步骤，时间线才算得出来。")
			empty.Children(func() {
				if ui.PrimaryButton(c, "去挑一道菜").Clicked() {
					a.router.Push("/recipes")
				}
			})
			return
		}

		ui.Row(c).FillWidth().Gap(design.Sp(c, 4)).Wrap().Children(func() {
			design.Stat(c, p, fmt.Sprintf("%d 分钟", tl.TotalMinutes), "并行后总时长")
			design.Stat(c, p, fmt.Sprintf("%d 分钟", tl.SerialMinutes), "一步一步做")
			design.Stat(c, p, fmt.Sprintf("%d 分钟", tl.SavedMinutes), "并行省下")
			design.Stat(c, p, fmt.Sprintf("%d 条", len(tl.Lanes)), "资源轨道")
		})

		a.timelineChart(c, p)
		a.timelineLegend(c, p)

		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Margin(16, 0, 0, 0).Children(func() {
			startLabel := "开始掌序"
			if a.sched.runActive {
				startLabel = "继续掌序"
			}
			if ui.PrimaryButton(c, startLabel).Clicked() {
				a.startOrResumeRun()
			}
			if ui.Button(c, "看备料清单").Clicked() {
				a.router.Push("/schedule/prep")
			}
			ui.Text(c, "块上的字是这道菜这一步，横向长度就是分钟数。").FontSize(11).TextColor(p.Faint)
		})
	})
}

// timelineChart is the Gantt: one track per lane, steps as proportional
// blocks. Percent widths keep it responsive without measuring the window.
func (a *App) timelineChart(c *ui.Context, p design.Palette) {
	tl := a.sched.timeline
	ui.Column(c).FillWidth().Gap(8).Padding(16).Radius(16).Background(p.Surface).
		Border(1, p.Border).Margin(0, 0, 12, 0).Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			ui.Box(c).Width(88).Shrink(0)
			ui.Row(c).Grow(1).Justify(ui.SpaceBetween).Children(func() {
				ui.Text(c, "0 分钟").FontSize(10).TextColor(p.Faint)
				ui.Text(c, fmt.Sprintf("%d 分钟", tl.TotalMinutes)).FontSize(10).TextColor(p.Faint)
			})
		})
		for _, lane := range tl.Lanes {
			lane := lane
			ui.Row(c).FillWidth().Height(44).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Row(c).Width(88).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Width(8).Height(8).Radius(999).Background(resourceColor(p, lane.Resource))
					ui.Text(c, lane.Label).FontSize(11).TextColor(p.Muted).SingleLine()
				})
				track := ui.Box(c).Grow(1).Height(34).Radius(8).Background(p.SurfaceAlt).Clip()
				track.Children(func() {
					ui.Row(c).FillWidth().Height(34).Children(func() {
						prev := 0
						for _, step := range lane.Steps {
							step := step
							if gap := step.Start - prev; gap > 0 {
								ui.Box(c).WidthPercent(percentOf(gap, tl.TotalMinutes)).Height(34)
							}
							block := ui.Box(c).WidthPercent(percentOf(step.Minutes(), tl.TotalMinutes)).
								Height(34).Radius(6).Clip().Center().
								Background(resourceColor(p, step.Resource)).
								Tooltip(fmt.Sprintf("%s · 第 %d 步 · %d 分钟",
									step.RecipeName, step.StepIndex, step.Minutes()))
							block.Children(func() {
								ui.Text(c, timelineBlockText(step)).FontSize(10).FontWeight(600).
									TextColor(p.OnAccent).SingleLine()
							})
							prev = step.End
						}
					})
				})
				ui.Text(c, fmt.Sprintf("%d–%d", lane.Start, lane.End)).FontSize(10).
					TextColor(p.Faint).Width(56).Shrink(0).TextAlign(ui.End)
			})
		}
	})
}

// timelineBlockText is the short label inside a step block.
func timelineBlockText(step domain.TimelineStep) string {
	label := fmt.Sprintf("%s·%d", firstRune(step.RecipeName), step.StepIndex)
	if step.Waiting {
		label += " 等"
	}
	return label
}

// timelineLegend explains the block colours.
func (a *App) timelineLegend(c *ui.Context, p design.Palette) {
	ui.Row(c).FillWidth().Wrap().Gap(14).Children(func() {
		for _, item := range []struct {
			resource string
			label    string
		}{
			{domain.ResourceStove, "灶台"},
			{domain.ResourceBoard, "案板"},
			{domain.ResourceHand, "动手"},
			{domain.ResourceWaiting, "等待（不占资源）"},
		} {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Width(9).Height(9).Radius(3).
					Background(resourceColor(p, item.resource))
				ui.Text(c, item.label).FontSize(11).TextColor(p.Muted)
			})
		}
	})
}

// resourceColor is the lane's colour.
func resourceColor(p design.Palette, resource string) ui.Color {
	switch resource {
	case domain.ResourceStove:
		return p.Accent
	case domain.ResourceBoard:
		return p.Warning
	case domain.ResourceWaiting:
		return p.Faint
	}
	return p.Success
}

// percentOf is v as a percentage of total, clamped into (0, 100].
func percentOf(v, total int) float32 {
	if total <= 0 || v <= 0 {
		return 0
	}
	p := float32(v) / float32(total) * 100
	if p > 100 {
		return 100
	}
	return p
}

// scheduleRunPage is the execution page: a control board on the left and the
// whole queue's steps as one lyric flow on the right, with the hanging
// countdown tray and the settlement.
func (a *App) scheduleRunPage(c *ui.Context, p design.Palette) {
	// Left/right walk the flow wherever the keyboard focus is.
	if c.Shortcut(0, ui.KeyRight) {
		a.runStepTo(a.sched.runStep + 1)
	}
	if c.Shortcut(0, ui.KeyLeft) {
		a.runStepTo(a.sched.runStep - 1)
	}
	if a.sched.runActive && len(a.sched.runLines) > 0 {
		a.runImmersive(c, p)
	} else {
		a.pageBody(c, func() {
			a.scheduleSubnav(c, p, "/schedule/run")
			a.runStartCard(c, p)
		})
	}
	a.settleSheet(c, p)

	// The run clock and the hanging countdowns tick once a second. When the
	// run is paused the frame does not ask for another one.
	if a.sched.runActive && !a.sched.runPaused {
		c.After(time.Second)
	}
}

// runImmersive is the run screen with the shell gone: the board and the
// steps fill the window, and the way out is the board's own control.
func (a *App) runImmersive(c *ui.Context, p design.Palette) {
	// The band a step is dropped on is not here: it is mounted over the
	// window by App.View, because a page inside the router is only as tall
	// as its content and an overlay anchored in it covers the wrong part of
	// the screen.
	col := ui.Column(c).FillWidth().Height(a.winH).MinWidth(0)
	col.Children(func() {
		a.runTopBar(c, p)
		a.runSplit(c, p)
	})
}

// runSplit is the 36/64 split: a compact control board and the steps.
func (a *App) runSplit(c *ui.Context, p design.Palette) {
	body := a.winH - design.TitleBand
	inner := body - design.Sp(c, 2.5) - design.Sp(c, 3)
	ui.Row(c).Grow(1).FillWidth().MinWidth(0).Height(body).
		AlignItems(ui.Stretch).
		Padding(design.Sp(c, 2.5), design.PagePadding, design.Sp(c, 3), design.PagePadding).
		Gap(design.Sp(c, 4)).Children(func() {
		ui.Column(c).Width(design.RunBoardWidth).Shrink(0).MinWidth(0).Children(func() {
			// The board and its hanging tray are one centred group; the
			// controls below are pinned to the bottom-left and do not take
			// part in the centring.
			ui.Column(c).Grow(1).Justify(ui.Center).Gap(design.Sp(c, 2)).Children(func() {
				a.hangTraySlot(c, p)
				a.runBoard(c, p)
			})
			a.runControls(c, p)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			a.runStage(c, p, inner)
		})
	})
}

// runTopBar is the immersive screen's own strip: one left arrow at the
// top-left ends the table, as the user asked. The rest drags the window.
func (a *App) runTopBar(c *ui.Context, p design.Palette) {
	bar := c.TitleBar()
	left := max(design.PagePadding, bar.Left+8)
	ui.Row(c).FillWidth().Height(design.TitleBand).
		Padding(0, max(16, bar.Right+12), 0, left).AlignItems(ui.Center).Children(func() {
		exit := ui.ButtonBase(c.Key("run-exit")).Size(32, 30).Radius(design.RadiusSmall).
			Center().Label("退出这桌")
		design.Wash(exit, p.Surface.Alpha(0), p.Hover)
		exit.Children(func() {
			ui.Icon(c, &icons.Back).Size(17, 17).TextColor(p.Ink)
		})
		if exit.Clicked() {
			a.abandonRun()
		}
		ui.Box(c).Grow(1).Height(design.TitleBand).DragWindow()
	})
}

// runTimerLabel is the run's clock, read where the current step is.
func (a *App) runTimerLabel() string {
	return clockText(a.sched.elapsed(time.Now().Unix()))
}

// hangDrag is a step being carried to the tray.
type hangDrag struct{ line int }

// hangDropRatio is how much of the screen the hanging band takes: the
// original's _kDropAreaRatio (app/lib/features/schedule/schedule_run_page.dart:109-110).
const hangDropRatio = 0.35

// hangDropZone is the band a step is dropped on to hang its countdown.
//
// It is built every frame and only shows while something is in the air,
// because the drop arrives on the frame the pointer is released — by then
// the handle no longer reports a drag, and a band that had unmounted with it
// would swallow the step. Out of a drag it is invisible and lets the pointer
// through, the way the dish page's sticky header does when hidden
// (recipes_views.go:594).
func (a *App) hangDropZone(c *ui.Context, p design.Palette) {
	_, h := c.Size()
	// All four edges are anchored, not just the top: inside a column an
	// absolute child with only a top and a height is laid out where the
	// flow had got to, and the band ends up under the steps instead of
	// over them.
	zone := ui.Box(c.Key("hang-drop")).Absolute().Top(design.TitleBand).Left(0).Right(0).
		Bottom(h * (1 - hangDropRatio)).Radius(design.RadiusPane).
		Label("挂起区")
	zone.IgnoreInput(!a.sched.dragging)
	zone.Opacity(boolFloat(a.sched.dragging))
	_, over := ui.DragOver[hangDrag](zone)
	if a.sched.dragging {
		zone.Background(p.Accent.Alpha(boolFloat(over) * 0.10))
		if over {
			zone.Border(1, p.Accent)
		}
		zone.Children(func() {
			ui.Column(c).FillWidth().Gap(6).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Icon(c, &icons.Clock).Size(20, 20).TextColor(p.Ink)
				label := "松手就把这一步挂起来计时"
				if len(a.sched.hang) > 0 {
					label = "松手就加进这一桌的计时"
				}
				ui.Text(c, label).FontSize(design.FontSmall).TextColor(p.Muted)
			})
		})
	}
	if drag, ok := ui.Drop[hangDrag](zone); ok {
		a.hangLine(drag.line)
	}
}

// runStartCard is the run page before it starts.
func (a *App) runStartCard(c *ui.Context, p design.Palette) {
	if len(a.schedule) == 0 {
		ui.Column(c).FillWidth().Padding(32).Gap(10).Radius(18).Background(p.Surface).
			Border(1, p.Border).Children(func() {
			ui.Text(c, "掌序是空的").FontSize(22).FontWeight(700).TextColor(p.Text)
			ui.Text(c, "先在菜式页挑一道，点「加入掌序」，再回来开火。").
				FontSize(13).TextColor(p.Muted).LineHeight(1.5)
			ui.Row(c).Gap(10).Margin(8, 0, 0, 0).Children(func() {
				if ui.PrimaryButton(c, "去挑一道菜").Clicked() {
					a.router.Push("/recipes")
				}
				if ui.Button(c, "回掌序").Clicked() {
					a.router.Replace("/schedule")
				}
			})
		})
		return
	}
	steps := a.sched.stepCount
	ui.Column(c).FillWidth().Padding(32).Gap(10).Radius(18).Background(p.Surface).
		Border(1, p.Border).Children(func() {
		ui.Text(c, "这桌还没开火").FontSize(22).FontWeight(700).TextColor(p.Text)
		if steps == 0 {
			ui.Text(c, "掌序里还没有带步骤的菜。先在菜式页加一道，再回来开火。").
				FontSize(13).TextColor(p.Muted).LineHeight(1.5)
			if ui.PrimaryButton(c, "去挑一道菜").Clicked() {
				a.router.Push("/recipes")
			}
			return
		}
		ui.Text(c, fmt.Sprintf("%d 道菜 · %d 步 · 并行排下来约 %d 分钟",
			len(a.schedule), steps, a.sched.timeline.TotalMinutes)).
			FontSize(13).TextColor(p.Muted).LineHeight(1.5)
		ui.Text(c, "开火后步骤会排成歌词流；等待类步骤可以挂起计时，关掉应用再回来还在这一步。").
			FontSize(12).TextColor(p.Faint).LineHeight(1.5)
		ui.Row(c).Gap(10).Margin(8, 0, 0, 0).Children(func() {
			if ui.PrimaryButton(c, "开始掌序").Clicked() {
				a.startRun()
			}
			if ui.Button(c, "看排程时间线").Clicked() {
				a.router.Push("/schedule/timeline")
			}
		})
	})
}

// runBoard is the left column's anchor: the run clock, the queue's own
// progress, and the table's nutrition at a glance.
func (a *App) runBoard(c *ui.Context, p design.Palette) {
	total := len(a.sched.runLines)
	frac := 0.0
	if total > 0 {
		frac = float64(a.sched.runStep+1) / float64(total)
	}
	card := ui.Column(c).FillWidth().MinHeight(360).Padding(design.Sp(c, 3.5)).
		Gap(design.Sp(c, 2)).Radius(design.RadiusCard).Background(p.Surface).
		Border(1, p.Border).AlignItems(ui.Start).Justify(ui.SpaceBetween).
		Label("掌序看板")
	design.RestShadow(card, p)
	minutes := a.sched.runTimeline.TotalMinutes
	if minutes == 0 {
		minutes = a.sched.timeline.TotalMinutes
	}
	title := "本桌料理看板"
	if a.sched.runMeal != "" {
		title = a.sched.runMeal + " · 本桌料理看板"
	}
	card.Children(func() {
		ui.Text(c, title).FontSize(design.FontMeta).FontWeight(700).
			TextColor(p.Muted).SingleLine()
		ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, a.runTimerLabel()).FontSize(design.FontHero).FontWeight(700).
				TextColor(p.Text).FontFeatures("tnum").SingleLine()
			ui.Text(c, fmt.Sprintf("/ 约 %d 分钟", minutes)).
				FontSize(design.FontSmall).TextColor(p.Muted).SingleLine()
		})
		ui.Progress(c, frac).FillWidth().Height(4).Label("掌序进度")
		ui.Divider(c)
		a.runDishProgress(c, p)
		ui.Divider(c)
		a.runNutritionSummary(c, p)
	})
}

// runDishProgress lists every dish on the table beside its own step count,
// so a multi-dish run never loses which pan is where.
func (a *App) runDishProgress(c *ui.Context, p design.Palette) {
	ui.Column(c).FillWidth().Gap(design.Sp(c, 1.25)).Children(func() {
		shown := 0
		for _, item := range a.schedule {
			rec, ok := a.recipeByID[item.RecipeID]
			if !ok {
				continue
			}
			steps, done := 0, 0
			for idx, line := range a.sched.runLines {
				if line.RecipeID != item.RecipeID {
					continue
				}
				steps++
				if idx <= a.sched.runStep {
					done++
				}
			}
			if steps == 0 {
				continue
			}
			shown++
			mark, color, status := "○", p.Faint, fmt.Sprintf("待开始 0/%d 步", steps)
			switch {
			case done >= steps:
				mark, color = "✓", p.Success
				status = fmt.Sprintf("已完成 %d/%d 步", steps, steps)
			case done > 0:
				mark, color = "●", p.Accent
				status = fmt.Sprintf("第 %d/%d 步", done, steps)
			}
			ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, mark).FontSize(design.FontCaption).TextColor(color)
				ui.Text(c, rec.Name).FontSize(design.FontSmall).FontWeight(600).
					TextColor(p.Text).Grow(1).SingleLine()
				ui.Text(c, status).FontSize(design.FontCaption).TextColor(p.Muted).SingleLine()
			})
		}
		if shown == 0 {
			ui.Text(c, "这桌还没有能执行的步骤").FontSize(design.FontSmall).TextColor(p.Muted)
		}
	})
}

// runNutritionSummary is the table's estimated energy, as one thin split
// bar and one line of grams.
func (a *App) runNutritionSummary(c *ui.Context, p design.Palette) {
	var kcal, prot, fat, carb float32
	have := false
	servingsOf := a.queueServings()
	for _, rec := range a.activeRunRecipes() {
		nut, ok := a.dishNut(rec)
		if !ok || !nut.Nutrients.HasData() {
			continue
		}
		have = true
		servings := float32(max(servingsOf[rec.ID], 1))
		kcal += float32(nut.Nutrients.Kcal) * servings
		prot += float32(nut.Nutrients.ProteinG) * servings
		fat += float32(nut.Nutrients.FatG) * servings
		carb += float32(nut.Nutrients.CarbG) * servings
	}
	if !have {
		ui.Text(c, "营养估算就绪后显示").FontSize(design.FontCaption).TextColor(p.Faint)
		return
	}
	total := prot*4 + fat*9 + carb*4
	ui.Column(c).FillWidth().Gap(design.Sp(c, 1)).Children(func() {
		ui.Text(c, fmt.Sprintf("合计 %.0f kcal · 每桌估算", kcal)).
			FontSize(design.FontSmall).FontWeight(600).TextColor(p.Text).SingleLine()
		bar := ui.Row(c).FillWidth().Height(6).Gap(2)
		bar.Children(func() {
			for _, seg := range []struct {
				v float32
				c ui.Color
			}{
				{prot * 4, p.Success}, {fat * 9, p.Warning}, {carb * 4, p.Accent},
			} {
				share := float32(0)
				if total > 0 {
					share = seg.v / total * 100
				}
				ui.Box(c).Height(6).WidthPercent(share).Radius(design.RadiusPill).
					Background(seg.c)
			}
		})
		ui.Row(c).FillWidth().Gap(design.Sp(c, 1.5)).Wrap().Children(func() {
			macroLegendItem(c, p, "蛋白", prot, p.Success)
			macroLegendItem(c, p, "脂肪", fat, p.Warning)
			macroLegendItem(c, p, "碳水", carb, p.Accent)
		})
	})
}

func macroLegendItem(c *ui.Context, p design.Palette, label string, grams float32, color ui.Color) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(5, 5).Radius(design.RadiusPill).Background(color)
		ui.Text(c, fmt.Sprintf("%s %.0f g", label, grams)).FontSize(design.FontCaption).
			TextColor(p.Muted).SingleLine()
	})
}

// runControls is Cloudy's transport: three icon-only buttons at the
// bottom-left. Labels stay for assistive technology and the tests; the exit
// lives at the top-left as a back arrow.
func (a *App) runControls(c *ui.Context, p design.Palette) {
	last := len(a.sched.runLines) - 1
	ui.Row(c).FillWidth().Gap(design.Sp(c, 2)).AlignItems(ui.Center).Children(func() {
		prev := ui.ButtonBase(c.Key("run-prev")).Size(design.RunControlSize, design.RunControlSize).
			Radius(design.RadiusPill).Center().Label("上一步")
		design.Wash(prev, p.Surface.Alpha(0), p.Hover)
		prev.Children(func() {
			ui.Icon(c, &icons.Prev).Size(17, 17).TextColor(p.Ink)
		})
		if a.sched.runStep <= 0 {
			prev.Disabled(true).Opacity(0.35)
		} else if prev.Clicked() {
			a.runStepTo(a.sched.runStep - 1)
		}

		pauseLabel, pauseIcon := "暂停", &icons.Pause
		if a.sched.runPaused {
			pauseLabel, pauseIcon = "继续", &icons.Play
		}
		pause := ui.ButtonBase(c.Key("run-pause")).Size(design.RunControlSize+8, design.RunControlSize+8).
			Radius(design.RadiusPill).Center().Label(pauseLabel).Background(p.Accent)
		if pause.Hovered() {
			pause.Background(p.Accent.Alpha(0.88))
		}
		pause.Children(func() {
			ui.Icon(c, pauseIcon).Size(18, 18).TextColor(p.OnAccent)
		})
		if pause.Clicked() {
			a.toggleRunPause()
		}

		nextLabel := "下一步"
		if last <= 0 || a.sched.runStep >= last {
			nextLabel = "结算"
		}
		next := ui.ButtonBase(c.Key("run-next")).Size(design.RunControlSize, design.RunControlSize).
			Radius(design.RadiusPill).Center().Label(nextLabel)
		design.Wash(next, p.Surface.Alpha(0), p.Hover)
		next.Children(func() {
			ui.Icon(c, &icons.Next).Size(17, 17).TextColor(p.Ink)
		})
		if next.Clicked() {
			if nextLabel == "结算" {
				a.openSettleConfirm()
			} else {
				a.runStepTo(a.sched.runStep + 1)
			}
		}
	})
}

// runStage is the lyric flow: no frame around the current step, only type
// weight and opacity, anchored a little above centre.
func (a *App) runStage(c *ui.Context, p design.Palette, height float32) {
	lines := make([]string, len(a.sched.runLines))
	for i, line := range a.sched.runLines {
		lines[i] = line.Text
	}
	// Whether a step is in the air is read as the rows are built, so the
	// band starts each frame knowing nothing.
	a.sched.dragging = false
	ui.Box(c).FillWidth().Height(height).Children(func() {
		a.lyricFlow(c, p, height, lines, lyricOptions{
			Anchor: 0.42,
			Head: func(i int) {
				a.runStepHead(c, p, i)
			},
			Tail: func(i int) {
				a.runStepControls(c, p, i)
			},
			Pick: func(i int) {
				a.runStepTo(i)
			},
		})
	})
}

// runStepHead names the dish and step above the lit line, as Cloudy labels
// the line it is singing.
func (a *App) runStepHead(c *ui.Context, p design.Palette, i int) {
	if i < 0 || i >= len(a.sched.runLines) {
		return
	}
	line := a.sched.runLines[i]
	ui.Text(c, fmt.Sprintf("%s · 第 %d 步", line.RecipeName, line.StepIndex)).
		FontSize(13).FontWeight(600).TextColor(p.Accent).SingleLine()
}

// runStepControls rides the lit row: a wait can be hung from here, and the
// one action that moves the run on. Nothing floats over another row.
func (a *App) runStepControls(c *ui.Context, p design.Palette, i int) {
	if i != a.sched.runStep || i < 0 || i >= len(a.sched.runLines) {
		return
	}
	line := a.sched.runLines[i]
	last := i >= len(a.sched.runLines)-1
	ui.Row(c).Gap(design.Sp(c, 1)).AlignItems(ui.Center).Shrink(0).Children(func() {
		if line.Waiting && !last {
			ui.Text(c, fmt.Sprintf("等 %d 分钟", max(line.Minutes, 1))).
				FontSize(design.FontMeta).TextColor(p.Muted).SingleLine()
			a.hangHandle(c, p, i)
		}
		label := "完成"
		if last {
			label = "结算"
		}
		// The key carries the line: a key shared across rows would let the
		// click that advances to the last step fire its 结算 in the same
		// frame.
		if design.Button(c, p, fmt.Sprintf("run-done-%d", i), label, design.ButtonPrimary).Clicked() {
			if last {
				a.openSettleConfirm()
			} else {
				a.runStepTo(i + 1)
			}
		}
	})
}

// hangTray geometry, shared with hangTraySlot so the animated height and the
// content cannot disagree.
const (
	hangTrayPad    float32 = 12
	hangTrayHeader float32 = 20
	hangTrayRow    float32 = 28
	hangTrayGap    float32 = 6
	hangTrayBottom float32 = 12
)

// hangTraySlot is the animated room above the board. Its height grows from
// zero when a countdown is hung and shrinks when the tray empties; because
// the group is centred with Justify, the board glides to its new centred
// place instead of jumping.
func (a *App) hangTraySlot(c *ui.Context, p design.Palette) {
	n := len(a.sched.hang)
	target := float32(0)
	if n > 0 {
		target = 2*hangTrayPad + hangTrayHeader + hangTrayGap +
			float32(n)*hangTrayRow + float32(n-1)*hangTrayGap + hangTrayBottom
	}
	slot := ui.Box(c.Key("hang-tray-slot")).FillWidth().Clip()
	k := target
	if a.level.Full() {
		k = slot.AnimateTo("hang-h", target, design.Pane)
	}
	shown := boolFloat(target > 0)
	op := shown
	if a.level.Moves() {
		op = slot.AnimateTo("hang-op", shown, design.Hover)
	}
	slot.Height(k).Opacity(op).IgnoreInput(k < 0.9)
	if n > 0 {
		slot.Children(func() {
			a.hangTray(c, p)
		})
	}
}

// hangTray shows the countdowns hung above the board: passive waits the cook
// can walk away from. It is narrow enough for the 300 DIP rail, so the
// per-step actions are icon-only.
func (a *App) hangTray(c *ui.Context, p design.Palette) {
	now := time.Now().Unix()
	tray := ui.Column(c).FillWidth().Gap(hangTrayGap).Padding(hangTrayPad).
		Radius(14).Background(p.SurfaceAlt).Border(1, p.Border).
		Margin(0, 0, hangTrayBottom, 0)
	if a.sched.dragging {
		tray.Border(2, p.Accent)
	}
	tray.Children(func() {
		ui.Row(c).FillWidth().Height(hangTrayHeader).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, &icons.Clock).Size(14, 14).TextColor(p.Ink)
			ui.Text(c, fmt.Sprintf("挂着 %d 步计时", len(a.sched.hang))).
				FontSize(11).FontWeight(700).TextColor(p.Text).Grow(1)
			clear := ui.ButtonBase(c.Key("hang-clear")).Height(18).Padding(1, 6).
				Radius(design.RadiusPill).Label("全部取下")
			design.Wash(clear, p.SurfaceAlt.Alpha(0), p.Hover)
			clear.Children(func() {
				ui.Text(c, "全部取下").FontSize(design.FontCaption).TextColor(p.Muted)
			})
			if clear.Clicked() {
				a.sched.hang = nil
			}
		})
		for i, hang := range a.sched.hang {
			i, hang := i, hang
			ui.Row(c).FillWidth().Height(hangTrayRow).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, hang.Title).FontSize(12).TextColor(p.Text).Grow(1).
					MinWidth(0).SingleLine()
				left := hang.remaining(now)
				color := p.Text
				text := clockText(left)
				if left == 0 {
					color = p.Success
					text = "时间到"
				}
				ui.Text(c, text).FontSize(12).FontWeight(700).TextColor(color).Shrink(0)
				pauseLabel, pauseIcon := "暂停", &icons.Pause
				if hang.Paused {
					pauseLabel, pauseIcon = "继续", &icons.Play
				}
				button := func(key string, icon *ui.SVG, label string, onClick func()) {
					b := ui.ButtonBase(c.Key(key)).Size(24, 24).Radius(design.RadiusPill).
						Center().Label(label)
					design.Wash(b, p.SurfaceAlt.Alpha(0), p.Hover)
					b.Children(func() {
						ui.Icon(c, icon).Size(12, 12).TextColor(p.Ink)
					})
					if b.Clicked() {
						onClick()
					}
				}
				button(fmt.Sprintf("hang-pause-%d", i), pauseIcon, pauseLabel,
					func() { a.toggleHangPause(i) })
				button(fmt.Sprintf("hang-drop-%d", i), &icons.Close, "取下",
					func() { a.unhang(hang.Line) })
			})
		}
	})
}

// settleSheet is the settlement confirmation: the dish count, the XP and the
// pantry rows the transaction will write.
func (a *App) settleSheet(c *ui.Context, p design.Palette) {
	if !a.sched.confirmOpen {
		return
	}
	preview := a.sched.preview
	title := "结算这桌"
	if a.sched.settleMeal != "" {
		title = "结算" + a.sched.settleMeal
	}
	design.ModalDialog(c, p, &a.sched.confirmOpen, 520, a.level, func() {
		ui.Text(c, title).FontSize(18).FontWeight(700).TextColor(p.Text)
		ui.Text(c, fmt.Sprintf("%d 道菜 · +%d XP · 这餐记录进食历，其余餐次保留",
			preview.Dishes, preview.XP)).FontSize(12).TextColor(p.Muted)
		if a.pantryOff {
			ui.Text(c, "仓库页已关闭，本餐不改动库存。").FontSize(12).TextColor(p.Muted)
		} else if preview.Deducted() > 0 {
			ui.Text(c, "库存扣减").FontSize(11).FontWeight(700).TextColor(p.Muted)
			ui.Column(c).FillWidth().Gap(5).Padding(12).Radius(12).
				Background(p.SurfaceAlt).Children(func() {
				shown := 0
				for _, change := range preview.Changes {
					if change.Skipped {
						continue
					}
					if shown >= 6 {
						continue
					}
					shown++
					ui.Text(c, change.Text()).FontSize(12).TextColor(p.Text)
				}
			})
		} else {
			ui.Text(c, "这桌的食材不在仓库或没有可用数量，库存不动。").
				FontSize(12).TextColor(p.Muted)
		}
		if preview.Skipped() > 0 {
			ui.Text(c, fmt.Sprintf("另有 %d 样没扣：不在仓库、没写用量或单位不同。",
				preview.Skipped())).FontSize(11).TextColor(p.Faint)
		}
		ui.Text(c, "精确数量按用量减；颗粒度降一档，扣到 0 标记用完；调料不动。").
			FontSize(11).TextColor(p.Faint).LineHeight(1.4)
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.sched.confirmOpen = false
			}
			if ui.PrimaryButton(c, "确认结算").Clicked() {
				a.settle()
			}
		})
	})
}
