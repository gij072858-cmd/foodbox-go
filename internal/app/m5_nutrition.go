package app

// M5 · 营养驾驶舱。
//
// This file is the nutrition workstream's home: the dashboard state and the
// nutrition page. The estimator itself lives in internal/services/nutrition.
//
// The page shows snapshots only. The one derived structure it reads
// (nutrition.IntakeReport) is rebuilt when the record slice changes, never on
// every frame, and never from a goroutine inside the view.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
	"github.com/gij072858-cmd/foodbox-go/internal/services/nutrition"
)

// nutritionState is the dashboard's view state: the daily target, the recent
// intake the page charts and the metric the contributor list is sorted by.
type nutritionState struct {
	// loaded is true once the embedded catalog and the body profile are read.
	loaded     bool
	catalog    *nutrition.Catalog
	energy     nutrition.Energy
	hasTarget  bool
	hasProfile bool
	goalLabel  string
	bodyLine   string
	acuteGout  bool
	report     nutrition.IntakeReport
	// sig is the record snapshot the report was built from.
	sig      nutritionSig
	computed bool
	// metric picks the contributor list: 0 calories, 1 protein, 2 purine.
	metric int
	// topList is the contributor table's virtual list state.
	topList ui.ListState
	// now pins the clock for tests and screenshots; zero means time.Now.
	now time.Time
}

// nowTime is the clock the dashboard buckets records by.
func (s *nutritionState) nowTime() time.Time {
	if s.now.IsZero() {
		return time.Now()
	}
	return s.now
}

// nutritionSig is a cheap fingerprint of the record slice, so the dashboard
// rebuilds its report only when the records changed.
type nutritionSig struct {
	records int
	firstID int64
	firstAt int64
	lastID  int64
}

func (s *nutritionState) init() {}

// nutritionProfileReader reads the self body profile through the same
// nineteen-column read the private kitchen uses. It is a variable so UI
// tests can inject an in-memory profile without a database.
var nutritionProfileReader = func(ctx context.Context, store repository.Repository) (domain.SichuProfile, bool, error) {
	if store == nil {
		return domain.SichuProfile{ExploreRatio: 0.3}, false, nil
	}
	return store.GetSichuProfile(ctx)
}

// loadNutritionState loads the nutrition catalog and the daily target. It runs
// on bootstrap's goroutine and publishes through a.post.
func (a *App) loadNutritionState(ctx context.Context, store repository.Repository) error {
	catalog, err := nutrition.NewCatalog()
	if err != nil {
		return err
	}
	profile, hasProfile, err := nutritionProfileReader(ctx, store)
	if err != nil {
		return err
	}
	in := nutritionEnergyInput(profile)
	energy := nutrition.EnergyFor(in)
	if hasProfile && profile.CalorieTarget > 0 {
		energy = nutrition.ApplyTarget(energy, in, profile.CalorieTarget)
	}
	a.post(func() {
		s := &a.nutrition
		s.catalog = catalog
		s.energy = energy
		s.hasTarget = energy.TargetKcal > 0
		s.hasProfile = hasProfile
		s.goalLabel = nutritionGoalLabel(in.Goals)
		s.bodyLine = nutritionBodyLine(profile, hasProfile)
		s.acuteGout = nutritionGoutAcute(profile)
		s.loaded = true
		s.computed = false
	})
	return nil
}

// ensureNutritionIntake rebuilds the dashboard report when the record
// snapshot changed. It touches at most a few hundred records and is a plain
// map lookup per ingredient, so it is safe to call from the page.
func (a *App) ensureNutritionIntake() {
	s := &a.nutrition
	if !s.loaded || s.catalog == nil {
		return
	}
	sig := nutritionRecordSignature(a.records)
	if s.computed && s.sig == sig {
		return
	}
	s.report = nutrition.AnalyzeIntake(a.records, a.recipeByID, s.catalog, s.nowTime())
	s.sig = sig
	s.computed = true
}

// nutritionRecordSignature fingerprints the ordered record slice.
func nutritionRecordSignature(records []domain.Record) nutritionSig {
	sig := nutritionSig{records: len(records)}
	if len(records) > 0 {
		sig.firstID = records[0].ID
		sig.firstAt = records[0].CompletedAt.Unix()
		sig.lastID = records[len(records)-1].ID
	}
	return sig
}

// nutritionEnergyInput maps the stored profile onto the estimator's input.
func nutritionEnergyInput(p domain.SichuProfile) nutrition.EnergyInput {
	return nutrition.EnergyInput{
		HeightCm:      p.HeightCm,
		WeightKg:      p.WeightKg,
		Age:           p.Age,
		Sex:           p.Sex,
		ActivityLevel: p.ActivityLevel,
		LifeStage:     p.LifeStage,
		Goals:         nutritionGoals(p),
	}
}

// nutritionGoals parses the goals column: a JSON array of labels, or the older
// single-goal column, or a comma separated string.
func nutritionGoals(p domain.SichuProfile) []string {
	var goals []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		var list []string
		if json.Unmarshal([]byte(raw), &list) == nil && len(list) > 0 {
			goals = append(goals, list...)
			return
		}
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '、' || r == '|' || r == ' '
		}) {
			if part != "" {
				goals = append(goals, part)
			}
		}
	}
	add(p.NutritionGoals)
	add(p.NutritionGoal)
	return goals
}

// nutritionGoalLabel is the short line the target card shows.
func nutritionGoalLabel(goals []string) string {
	if len(goals) == 0 {
		return "均衡"
	}
	return strings.Join(goals, " · ")
}

// nutritionBodyLine is the one-line profile summary under the target.
func nutritionBodyLine(p domain.SichuProfile, has bool) string {
	if !has {
		return ""
	}
	parts := make([]string, 0, 5)
	if p.HeightCm > 0 {
		parts = append(parts, fmt.Sprintf("%d cm", p.HeightCm))
	}
	if p.WeightKg > 0 {
		parts = append(parts, fmt.Sprintf("%d kg", p.WeightKg))
	}
	if p.Age > 0 {
		parts = append(parts, fmt.Sprintf("%d 岁", p.Age))
	}
	switch {
	case strings.Contains(strings.ToLower(p.Sex), "male"), strings.Contains(p.Sex, "男"):
		parts = append(parts, "男")
	case strings.Contains(strings.ToLower(p.Sex), "female"), strings.Contains(p.Sex, "女"):
		parts = append(parts, "女")
	}
	if p.ActivityLevel != "" {
		parts = append(parts, domain.ActivityLabel(p.ActivityLevel))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

// nutritionGoutAcute reports whether the profile marks an acute gout flare,
// which is the only case where purine has a hard ceiling.
func nutritionGoutAcute(p domain.SichuProfile) bool {
	stage := p.HealthStages + " " + p.NutritionGoals + " " + p.NutritionGoal
	return strings.Contains(stage, "goutAcute") || strings.Contains(stage, "急性")
}

// nutritionPage is the dashboard.
func (a *App) nutritionPage(c *ui.Context, p design.Palette) {
	a.ensureNutritionIntake()
	s := &a.nutrition
	a.pageBody(c, func() {
		if !s.loaded || s.catalog == nil {
			design.EmptyState(c, p, &icons.Bowl, "成分表还没有加载",
				"营养估算依赖随包的食物成分表；重新进入此页再试。").FillWidth()
			return
		}
		a.nutritionTargetCard(c, p)
		if s.report.Week.Dishes == 0 {
			a.nutritionEmptyCard(c, p)
			return
		}
		a.nutritionTodayCard(c, p)
		a.nutritionWeekCard(c, p)
		a.nutritionPurineCard(c, p)
		a.nutritionTopCard(c, p)
		ui.Text(c, "以上数值都由随包食物成分表与家常用量估算，不构成医学建议。").
			FontSize(11).TextColor(p.Faint).Margin(4, 8, 0, 0)
	})
}

// nutritionTargetCard shows BMR, TDEE, target calories and the macros.
func (a *App) nutritionTargetCard(c *ui.Context, p design.Palette) {
	s := &a.nutrition
	card := ui.Column(c).FillWidth().Gap(design.Sp(c, 2)).Margin(0, 0, design.Sp(c, 4), 0)
	card.Children(func() {
		design.HeaderAction(c, p, "今日目标", "估算 · 按身体档案与活动量", func() {
			if ui.Button(c, "补全身体档案").Clicked() {
				a.router.Push("/sichu/body")
			}
			if ui.Button(c, "去私厨配餐").Clicked() {
				a.router.Push("/sichu")
			}
		})
		if !s.hasTarget {
			ui.Text(c, "目标热量还不可用").FontSize(15).FontWeight(700).TextColor(p.Text)
			note := s.energy.Note
			if note == "" {
				note = "身体档案还没填，缺身高、体重或年龄。"
			}
			ui.Text(c, note+"补齐后这里会给出 BMR、TDEE 与蛋白、脂肪、碳水的目标克数。").
				FontSize(12).TextColor(p.Muted).LineHeight(1.5)
			if ui.PrimaryButton(c, "去填身体档案").Clicked() {
				a.router.Push("/sichu/body")
			}
			return
		}
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			design.StatCard(c, p, fmt.Sprintf("%d", s.energy.BMR), "基础代谢 BMR · kcal")
			design.StatCard(c, p, fmt.Sprintf("%d", s.energy.TDEE), "日常消耗 TDEE · kcal")
			design.StatCard(c, p, fmt.Sprintf("%d", s.energy.TargetKcal), "目标热量 · kcal")
		})
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			design.StatCard(c, p, fmt.Sprintf("%d g", s.energy.ProteinG), "蛋白 · 目标")
			design.StatCard(c, p, fmt.Sprintf("%d g", s.energy.FatG), "脂肪 · 目标")
			design.StatCard(c, p, fmt.Sprintf("%d g", s.energy.CarbG), "碳水 · 目标")
		})
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "营养目标："+s.goalLabel).FontSize(12).TextColor(p.Muted)
			if s.bodyLine != "" {
				ui.Text(c, "·").FontSize(12).TextColor(p.Faint)
				ui.Text(c, s.bodyLine).FontSize(12).TextColor(p.Muted)
			}
		})
		if s.energy.Note != "" {
			ui.Text(c, "提示："+s.energy.Note).FontSize(11).TextColor(p.Faint).LineHeight(1.4)
		}
	})
}

// nutritionEmptyCard is the way out when there is nothing to chart yet.
func (a *App) nutritionEmptyCard(c *ui.Context, p design.Palette) {
	card := design.EmptyState(c, p, &icons.Bowl, "还没有开饭记录",
		"在掌序里结算一道菜，或直接做一道菜，这里会按食物成分表估算摄入，并按天画出趋势。")
	card.FillWidth().Children(func() {
		ui.Row(c).Gap(10).Margin(0, 10, 0, 0).Children(func() {
			if ui.PrimaryButton(c, "去挑一道菜").Clicked() {
				a.router.Push("/recipes")
			}
			if ui.Button(c, "去掌序结算").Clicked() {
				a.router.Push("/schedule")
			}
			if ui.Button(c, "看食历").Clicked() {
				a.router.Push("/feed")
			}
		})
	})
}

// nutritionTodayCard shows today's intake against the target.
func (a *App) nutritionTodayCard(c *ui.Context, p design.Palette) {
	s := &a.nutrition
	today := s.report.Today
	card := ui.Column(c).FillWidth().Gap(design.Sp(c, 2)).Margin(0, 0, design.Sp(c, 4), 0)
	card.Children(func() {
		design.SectionHeader(c, p, "今日摄入", "估算 · 每道菜按一份计")
		if s.hasTarget {
			ratio := 0.0
			if s.energy.TargetKcal > 0 {
				ratio = today.Kcal / float64(s.energy.TargetKcal)
			}
			ui.Progress(c, ratio).FillWidth()
			ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				ui.Text(c, fmt.Sprintf("%s / %d kcal", nutritionNumber(today.Kcal), s.energy.TargetKcal)).
					FontSize(13).FontWeight(600).TextColor(p.Text)
				ui.Text(c, nutritionRemainingLabel(today.Kcal, float64(s.energy.TargetKcal))).
					FontSize(12).TextColor(p.Muted)
			})
			a.nutritionMacroRow(c, p, "蛋白", today.ProteinG, float64(s.energy.ProteinG), p.Success)
			a.nutritionMacroRow(c, p, "脂肪", today.FatG, float64(s.energy.FatG), p.Warning)
			a.nutritionMacroRow(c, p, "碳水", today.CarbG, float64(s.energy.CarbG), p.Accent)
		} else {
			ui.Text(c, fmt.Sprintf("今日已摄入约 %s kcal", nutritionNumber(today.Kcal))).
				FontSize(15).FontWeight(600).TextColor(p.Text)
			ui.Text(c, "补全身体档案后，这里会出现目标进度与宏量条。").
				FontSize(12).TextColor(p.Muted)
			if ui.Button(c, "补全身体档案").Clicked() {
				a.router.Push("/sichu/body")
			}
		}
		if line := nutritionCoverageLine(today); line != "" {
			ui.Text(c, line).FontSize(11).TextColor(p.Faint).LineHeight(1.4)
		}
		if today.PurineMg > 0 {
			ui.Text(c, fmt.Sprintf("今日嘌呤约 %s mg（估算）", nutritionNumber(today.PurineMg))).
				FontSize(11).TextColor(p.Faint)
		}
	})
}

// nutritionMacroRow is one labelled meter with its numbers.
func (a *App) nutritionMacroRow(c *ui.Context, p design.Palette, label string, done, target float64, color ui.Color) {
	ratio := 0.0
	if target > 0 {
		ratio = done / target
	}
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(6, 6).Radius(design.RadiusPill).Background(color).Shrink(0)
		ui.Text(c, label).FontSize(12).TextColor(p.Muted).Width(36).Shrink(0)
		ui.Progress(c, ratio).Grow(1)
		ui.Text(c, fmt.Sprintf("%s / %s g", nutritionNumber(done), nutritionNumber(target))).
			FontSize(11).TextColor(p.Text).Width(92).Shrink(0).TextAlign(ui.End)
	})
}

// nutritionWeekCard charts the last seven days.
func (a *App) nutritionWeekCard(c *ui.Context, p design.Palette) {
	s := &a.nutrition
	card := ui.Column(c).FillWidth().Gap(design.Sp(c, 2)).Margin(0, 0, design.Sp(c, 4), 0)
	card.Children(func() {
		design.SectionHeader(c, p, "最近 7 天", "估算 · 按本地日期汇总")
		labels := make([]string, len(s.report.Days))
		values := make([]float64, len(s.report.Days))
		for i, day := range s.report.Days {
			labels[i] = day.Date.Format("01-02")
			values[i] = day.Kcal
		}
		design.BarChart(c, p, labels, values, 132)
		days := 0
		max := 0.0
		for _, day := range s.report.Days {
			if day.Dishes > 0 {
				days++
			}
			if day.Kcal > max {
				max = day.Kcal
			}
		}
		avg := 0.0
		if days > 0 {
			avg = s.report.Week.Kcal / float64(days)
		}
		ui.Row(c).FillWidth().Gap(18).Children(func() {
			ui.Text(c, fmt.Sprintf("有记录 %d 天", days)).FontSize(12).TextColor(p.Muted)
			ui.Text(c, fmt.Sprintf("日均约 %s kcal", nutritionNumber(avg))).FontSize(12).TextColor(p.Muted)
			ui.Text(c, fmt.Sprintf("最高约 %s kcal", nutritionNumber(max))).FontSize(12).TextColor(p.Muted)
		})
		ui.Text(c, fmt.Sprintf("7 天共记录 %d 道菜，合计约 %s kcal（估算）",
			s.report.Week.Dishes, nutritionNumber(s.report.Week.Kcal))).
			FontSize(11).TextColor(p.Faint)
	})
}

// nutritionPurineCard shows the day's purine load.
func (a *App) nutritionPurineCard(c *ui.Context, p design.Palette) {
	s := &a.nutrition
	today := s.report.Today
	card := ui.Column(c).FillWidth().Gap(design.Sp(c, 2)).Margin(0, 0, design.Sp(c, 4), 0)
	card.Children(func() {
		header := ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween)
		header.Children(func() {
			design.SectionHeader(c, p, "嘌呤负荷", "估算 · 痛风参考")
			if level := s.report.FocusPurine; level.Severity() > 0 {
				design.Tag(c, p, "最高 "+string(level), nutritionPurineColor(p, level))
			}
		})
		note := nutrition.PurineNote(today.PurineMg, today.PurineMissing, s.acuteGout)
		color := p.Muted
		if s.acuteGout && today.PurineMg > nutrition.AcutePurineCapMg {
			color = p.Danger
		}
		ui.Text(c, note).FontSize(13).TextColor(color).LineHeight(1.5)
		ui.Meter(c, today.PurineMg, 0, 150,
			&ui.MeterLevels{Warning: 50, Critical: 150}).Label("嘌呤负荷")
		ui.Text(c, "分档口径：低 <50 / 中 50–150 / 高 ≥150 mg 每 100 g；表里没有数据的菜不计入。").
			FontSize(11).TextColor(p.Faint).LineHeight(1.4)
	})
}

// nutritionTopCard lists the day's biggest ingredient contributions.
func (a *App) nutritionTopCard(c *ui.Context, p design.Palette) {
	s := &a.nutrition
	card := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
	card.Children(func() {
		design.HeaderAction(c, p, "食材贡献", "估算 · 每份计，调料计入", func() {
			ui.Segmented(c, &s.metric, "按热量", "按蛋白", "按嘌呤").Changed()
		})
		rows, unit, color := nutritionContributorView(s, p)
		if len(rows) == 0 {
			ui.Text(c, "这一天的记录还不足以拆分食材贡献。").
				FontSize(12).TextColor(p.Muted)
			return
		}
		focus := "今天记录的食材"
		if s.report.Focus.Format("2006-01-02") != s.nowTime().Format("2006-01-02") {
			focus = s.report.Focus.Format("01-02") + " 记录的食材"
		}
		ui.Text(c, focus+"按"+unit+"排序，取前 "+fmt.Sprint(len(rows))+" 位。").
			FontSize(11).TextColor(p.Faint)
		cols := []ui.TableColumn{
			{Title: "食材"},
			{Title: "份量", Width: 96, Align: ui.End},
			{Title: unit, Width: 104, Align: ui.End},
		}
		h := float32(len(rows)*44 + 48)
		if h > 420 {
			h = 420
		}
		ui.Table(c.Key("nutrition-top"), &s.topList, cols, len(rows), func(row, col int) {
			r := rows[row]
			switch col {
			case 0:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprint(row+1)).FontSize(11).FontWeight(700).
						TextColor(p.Faint).Width(16).Shrink(0)
					ui.Text(c, r.Name).FontSize(13).FontWeight(600).
						TextColor(p.Text).SingleLine()
				})
			case 1:
				ui.Text(c, fmt.Sprintf("约 %s g", nutritionGrams(r.ServedGrams))).
					FontSize(11).TextColor(p.Muted)
			case 2:
				ui.Text(c, nutritionContributorValue(r, s.metric)).
					FontSize(13).FontWeight(600).TextColor(color)
			}
		}).Height(h)
	})
}

// nutritionContributorView returns the list for the selected metric with its
// unit and colour.
func nutritionContributorView(s *nutritionState, p design.Palette) ([]nutrition.Contribution, string, ui.Color) {
	switch s.metric {
	case 1:
		return s.report.TopProtein, "蛋白", p.Success
	case 2:
		return s.report.TopPurine, "嘌呤", p.Warning
	}
	return s.report.TopKcal, "热量", p.Accent
}

// nutritionContributorValue is the trailing number of a contributor row.
func nutritionContributorValue(row nutrition.Contribution, metric int) string {
	switch metric {
	case 1:
		return fmt.Sprintf("%s g", nutritionGrams(row.ProteinG))
	case 2:
		return fmt.Sprintf("%s mg", nutritionNumber(row.PurineMg))
	}
	return fmt.Sprintf("%s kcal", nutritionNumber(row.Kcal))
}

// nutritionCoverageLine describes how much of the day's food the table
// actually covered.
func nutritionCoverageLine(day nutrition.DayIntake) string {
	if day.Dishes == 0 {
		return "今天还没有开饭记录"
	}
	parts := []string{fmt.Sprintf("%d 道菜", day.Dishes)}
	if day.Full > 0 {
		parts = append(parts, fmt.Sprintf("%d 道成分齐全", day.Full))
	}
	if day.Partial > 0 {
		parts = append(parts, fmt.Sprintf("%d 道部分估算", day.Partial))
	}
	if day.Uncovered > 0 {
		parts = append(parts, fmt.Sprintf("%d 道缺成分数据", day.Uncovered))
	}
	return "今天已估算 " + strings.Join(parts, "，") + "；调料按实际用量计入。"
}

// nutritionRemainingLabel is "还差 X kcal" or "已超出 X kcal".
func nutritionRemainingLabel(done, target float64) string {
	if target <= 0 {
		return ""
	}
	if done <= target {
		return fmt.Sprintf("还可约 %s kcal", nutritionNumber(target-done))
	}
	return fmt.Sprintf("已超出约 %s kcal", nutritionNumber(done-target))
}

// nutritionPurineColor colours a purine tier tag.
func nutritionPurineColor(p design.Palette, level nutrition.PurineLevel) ui.Color {
	switch level {
	case nutrition.PurineHigh:
		return p.Danger
	case nutrition.PurineMedium:
		return p.Warning
	case nutrition.PurineLow:
		return p.Success
	}
	return p.Muted
}

// nutritionNumber formats an estimate: no decimals, thousands kept plain.
func nutritionNumber(v float64) string {
	return fmt.Sprintf("%.0f", math.Round(v))
}

// nutritionGrams formats a weight with one decimal below 10 g.
func nutritionGrams(v float64) string {
	if v < 10 {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.0f", math.Round(v))
}
