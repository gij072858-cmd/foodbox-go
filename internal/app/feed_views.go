package app

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// feedPage is the food calendar: the month grid, the streak stats and the
// latest meals. The grid and the review come from feedState, which is rebuilt
// off the frame.
func (a *App) feedPage(c *ui.Context, p design.Palette) {
	a.feed.ensure(a)
	if len(a.feed.records) == 0 {
		a.feedEmptyState(c, p)
		return
	}
	a.pageFrame(c, func() {
		ui.Split(c, &a.feed.splitWidth, func() {
			a.feedMonthPane(c, p)
		}, func() {
			a.feedDayPane(c, p)
		}).Grow(1)
	})
}

// feedEmptyState is the whole page when nothing has been cooked: one panel,
// one action. A calendar of empty months and a row of zeroes would only be
// the same fact three times.
func (a *App) feedEmptyState(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		box := design.EmptyState(c, p, &icons.Feed, "还没有开饭记录",
			"去掌序做一道菜，结算之后回来这里就有第一天。")
		box.Width(420).AlignSelf(ui.Center).Margin(0, 0, 24, 0)
		box.Children(func() {
			if ui.PrimaryButton(c, "去掌序做一道菜").Clicked() {
				a.router.Push("/schedule")
			}
		})
	})
}

// feedMonthPane is the left column: the month, and one quiet line that
// says how many days and dishes there are and how long the streak is.
func (a *App) feedMonthPane(c *ui.Context, p design.Palette) {
	ui.Column(c).FillWidth().Gap(design.Sp(c, 2.5)).Padding(0, design.PagePadding, 0, 0).Children(func() {
		if ui.Calendar(c, &a.feed.date).Changed() {
			// The framework's calendar moved the chosen day; the month's
			// summary follows it off the frame.
			a.feed.year, a.feed.month = a.feed.date.Year(), int(a.feed.date.Month())
			a.feed.buildMonth()
		}
		ui.Text(c, fmt.Sprintf("%d 天 · %d 道 · 连续 %d 天",
			a.mealDays, a.recordCount, a.feed.streak)).
			FontSize(design.FontMeta).TextColor(p.Muted)
		ui.Text(c, fmt.Sprintf("本月开饭 %d 天", a.feedMonthDays())).
			FontSize(design.FontMeta).TextColor(p.Faint)
		ui.Divider(c)
		ui.Row(c).Gap(design.Sp(c, 1.5)).Children(func() {
			if ui.Button(c, "写这一天的日记").Clicked() {
			date := a.feed.date.Format("2006-01-02")
			a.openFeedDay(date)
				a.router.Push("/feed/" + date)
			}
			if ui.Button(c, "年度回顾").Clicked() {
				a.router.Push("/feed/review")
			}
		})
	})
}

// feedMonthDays counts days with a record in the month on screen.
func (a *App) feedMonthDays() int {
	n := 0
	for _, week := range a.feed.weeks {
		for _, day := range week {
			if day != nil && day.Count > 0 {
				n++
			}
		}
	}
	return n
}

// feedDayPane is the right column: the chosen day's meals, and the latest
// meals under them. It is rebuilt each frame from the snapshot, and only
// reads the date the calendar keeps.
func (a *App) feedDayPane(c *ui.Context, p design.Palette) {
	date := a.feed.date.Format("2006-01-02")
	title, weekday := feedDayTitle(date)
	ui.Scroll(c).Fill().MinWidth(0).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 0, 0, design.PagePadding).Gap(design.Sp(c, 2.5)).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Baseline).Gap(design.Sp(c, 1.5)).Children(func() {
				ui.Text(c, title).FontSize(design.FontSubhead).FontWeight(700).TextColor(p.Text).Role(ui.RoleHeading)
				ui.Text(c, weekday).FontSize(design.FontMeta).TextColor(p.Muted)
			})
			if a.feedDayRecords(date) == 0 {
				ui.Text(c, "这一天没有结算记录。").FontSize(design.FontSmall).TextColor(p.Muted)
			} else {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					for _, r := range a.feed.records {
						if r.CompletedAt.In(time.Local).Format("2006-01-02") != date {
							continue
						}
						a.feedRecordRow(c, p, r)
					}
				})
			}
			if note := a.feed.journals[date].Note; note != "" {
				ui.Text(c, "日记："+note).FontSize(design.FontMeta).TextColor(p.Muted).LineHeight(1.5)
			}

			a.section(c, p, "最近开饭", func() {
				shown := 0
				last := ""
				for _, r := range a.feed.records {
					if shown >= 8 {
						break
					}
					day := r.CompletedAt.In(time.Local).Format("2006-01-02")
					if day != last {
						last = day
						ui.Text(c, feedDateLabel(day)).FontSize(design.FontMeta).FontWeight(700).
							TextColor(p.Muted).Margin(design.Sp(c, 1), 0, 0, 0)
					}
					shown++
					a.feedRecordRow(c, p, r)
				}
				if ui.Button(c, "看全部做过的菜").Clicked() {
					a.router.Push("/profile/cooked")
				}
			})
		})
	})
}

// feedDayRecords counts the chosen day's settlements.
func (a *App) feedDayRecords(date string) int {
	n := 0
	for _, r := range a.feed.records {
		if r.CompletedAt.In(time.Local).Format("2006-01-02") == date {
			n++
		}
	}
	return n
}

func (a *App) feedRecordRow(c *ui.Context, p design.Palette, r domain.Record) {
	ui.Row(c).FillWidth().Gap(design.Sp(c, 1.5)).Padding(design.Sp(c, 1.5), design.Sp(c, 1)).
		Radius(design.RadiusSmall).AlignItems(ui.Center).Role(ui.RoleListItem).Children(func() {
		ui.Text(c, r.CompletedAt.In(time.Local).Format("15:04")).FontSize(design.FontMeta).
			TextColor(p.Faint).Width(48)
		ui.Text(c, a.recipeName(r.RecipeID)).FontSize(design.FontSmall).
			FontWeight(600).TextColor(p.Text).Grow(1)
		if d := recordDuration(r); d != "" {
			ui.Text(c, d).FontSize(design.FontMeta).TextColor(p.Muted)
		}
		ui.Text(c, fmt.Sprintf("+%d XP", r.XPGained)).FontSize(design.FontMeta).TextColor(p.Accent)
	})
}


// feedDayBody draws one day's journal editor.
func (a *App) feedDayBody(c *ui.Context, p design.Palette) {
	d := &a.feed.day
	title, weekday := feedDayTitle(d.date)
	a.pageBody(c, func() {
		a.subHeader(c, p, title, weekday)

		a.section(c, p, "当天记录", func() {
			if len(d.records) == 0 {
				ui.Column(c).FillWidth().Padding(18).Gap(4).Radius(14).
					Background(p.Surface).Border(1, p.Border).Children(func() {
					ui.Text(c, "这一天没有结算记录").FontSize(13).FontWeight(600).TextColor(p.Text)
					ui.Text(c, "备注和照片仍然可以写，做完菜结算后记录会自动出现在这里。").FontSize(12).TextColor(p.Muted)
				})
				return
			}
			ui.Column(c).FillWidth().Gap(1).Radius(14).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				for _, r := range d.records {
					r := r
					ui.Row(c).FillWidth().Padding(12, 10).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, r.CompletedAt.In(time.Local).Format("15:04")).FontSize(11).
							TextColor(p.Faint).Width(48)
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, a.recipeName(r.RecipeID)).FontSize(13).
								FontWeight(600).TextColor(p.Text)
							if r.Note != "" {
								ui.Text(c, r.Note).FontSize(11).TextColor(p.Muted)
							}
						})
						if d := recordDuration(r); d != "" {
							ui.Text(c, d).FontSize(11).TextColor(p.Muted)
						}
						ui.Text(c, fmt.Sprintf("+%d XP", r.XPGained)).FontSize(11).TextColor(p.Accent)
					})
				}
			})
		})

		a.section(c, p, "备注", func() {
			box := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
			box.Children(func() {
				design.FieldLabel(c, p, "这一天的想法")
				ui.TextArea(c, &a.feed.day.note).Lines(3, 6).
					Placeholder("记下这一餐的味道、改动，或者下次要少放盐。")
			})
		})

		a.section(c, p, "照片", func() {
			box := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
			box.Children(func() {
				ui.Text(c, "桌面版只记录照片路径，不复制图片，也不参与备份。").FontSize(12).TextColor(p.Muted)
				if len(d.photos) > 0 {
					ui.Column(c).FillWidth().Gap(6).Children(func() {
						for i, path := range d.photos {
							i, path := i, path
							ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
								ui.Text(c, filepath.Base(path)).FontSize(12).TextColor(p.Text).Grow(1).SingleLine()
								if ui.Button(c, "移除").Clicked() {
									a.removeJournalPhoto(i)
								}
							})
						}
					})
				}
				ui.Row(c).Gap(10).Children(func() {
					if ui.Button(c, "选择照片").Clicked() {
						a.pickJournalPhoto()
					}
				})
			})
		})

		ui.Row(c).Gap(10).Children(func() {
			if ui.PrimaryButton(c, "保存日记").Clicked() {
				a.saveFeedDay()
			}
			if ui.Button(c, "回到食历").Clicked() {
				a.router.Push("/feed")
			}
		})
	})
}

// feedReviewBody draws the annual review.
func (a *App) feedReviewBody(c *ui.Context, p design.Palette) {
	f := &a.feed
	stats := f.reviewStats
	a.pageBody(c, func() {
		a.subHeader(c, p, fmt.Sprintf("%d 年度回顾", f.reviewYear),
			"这一年的开饭记录与口味")

		if len(f.years) > 1 {
			idx := 0
			labels := make([]string, len(f.years))
			for i, year := range f.years {
				labels[i] = fmt.Sprintf("%d 年", year)
				if year == f.reviewYear {
					idx = i
				}
			}
			if design.Segmented(c, &idx, labels...).Label("回顾年份").Changed() &&
				idx >= 0 && idx < len(f.years) {
				a.setReviewYear(f.years[idx])
			}
		}

		if len(f.records) == 0 {
			box := design.EmptyState(c, p, &icons.Chart, "还没有可以回顾的记录",
				"先去做一道菜，结算之后这里会有月度分布、最常做的菜和菜系比例。")
			box.Children(func() {
				if ui.PrimaryButton(c, "去掌序做一道菜").Clicked() {
					a.router.Push("/schedule")
				}
			})
			return
		}

		ui.Row(c).FillWidth().Gap(design.Sp(c, 4)).Wrap().Children(func() {
			design.Stat(c, p, fmt.Sprint(stats.Dishes), "总菜数")
			design.Stat(c, p, fmt.Sprint(stats.Days), "开饭天数")
			design.Stat(c, p, fmt.Sprint(stats.LongestStreak), "最长连续")
			avg := 0.0
			if stats.Days > 0 {
				avg = float64(stats.Dishes) / float64(stats.Days)
			}
			design.Stat(c, p, fmt.Sprintf("%.1f", avg), "每天菜数")
		})

		a.section(c, p, "月度开饭", func() {
			box := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
			box.Children(func() {
				labels := make([]string, 12)
				values := make([]float64, 12)
				for i := 0; i < 12; i++ {
					labels[i] = fmt.Sprint(i + 1)
					values[i] = float64(stats.Months[i])
				}
				design.BarChart(c, p, labels, values, 150)
				ui.Text(c, "横轴是月份，柱高是当月做的菜数。").FontSize(11).TextColor(p.Faint)
			})
		})

		a.section(c, p, "最常做", func() {
			if len(stats.Top) == 0 {
				ui.Text(c, "这一年还没有记录。").FontSize(12).TextColor(p.Muted)
				return
			}
			box := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
			box.Children(func() {
				top := stats.Top[0].Count
				for i, dish := range stats.Top {
					i, dish := i, dish
					ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Text(c, fmt.Sprint(i+1)).FontSize(12).FontWeight(700).TextColor(p.Accent).
							Width(20).TextAlign(ui.Center)
						ui.Text(c, dish.Name).FontSize(13).TextColor(p.Text).Grow(1).SingleLine()
						ui.Text(c, fmt.Sprintf("%d 次", dish.Count)).FontSize(11).TextColor(p.Muted)
					})
					fraction := float32(0)
					if top > 0 {
						fraction = float32(dish.Count) / float32(top)
					}
					ui.Meter(c, float64(fraction), 0, 1, nil).Label(dish.Name + " 的次数占比")
				}
			})
		})

		a.section(c, p, "菜系分布", func() {
			if len(stats.Cuisines) == 0 {
				ui.Text(c, "还没有菜系数据。").FontSize(12).TextColor(p.Muted)
				return
			}
			box := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
			box.Children(func() {
				top := stats.Cuisines[0].Count
				for _, slice := range stats.Cuisines {
					slice := slice
					ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Text(c, slice.Label).FontSize(12).TextColor(p.Text).Grow(1).SingleLine()
						ui.Text(c, fmt.Sprintf("%d 道", slice.Count)).FontSize(11).TextColor(p.Muted)
					})
					fraction := float32(0)
					if top > 0 {
						fraction = float32(slice.Count) / float32(top)
					}
					ui.Meter(c, float64(fraction), 0, 1, nil).Label(slice.Label + " 的菜式占比")
				}
			})
		})
	})
}

// recipeName reads a dish name from the loaded recipe snapshot, keeping the
// history readable after the dish itself was deleted.
func (a *App) recipeName(id int64) string {
	if rec, ok := a.recipeByID[id]; ok && rec.Name != "" {
		return rec.Name
	}
	if rec, ok := a.feed.recipes[id]; ok && rec.Name != "" {
		return rec.Name
	}
	return "已删除的菜式"
}

// feedDateLabel writes a day without the year when it is this year.
func feedDateLabel(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	if t.Year() == time.Now().Year() {
		return fmt.Sprintf("%d 月 %d 日", t.Month(), t.Day())
	}
	return fmt.Sprintf("%d 年 %d 月 %d 日", t.Year(), t.Month(), t.Day())
}

// feedDayTitle writes the day page's heading and its weekday.
func feedDayTitle(date string) (string, string) {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return date, ""
	}
	weekdays := [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return fmt.Sprintf("%d 年 %d 月 %d 日", t.Year(), t.Month(), t.Day()),
		weekdays[int(t.Weekday())]
}

// feedDuration renders a session length in minutes.
func feedDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	return fmt.Sprintf("%d 分钟", seconds/60)
}

// recordDuration renders one record's session length.
func recordDuration(r domain.Record) string {
	if r.SessionSeconds == nil || *r.SessionSeconds <= 0 {
		return ""
	}
	return feedDuration(*r.SessionSeconds)
}
