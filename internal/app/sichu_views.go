package app

// The private kitchen's workbench: a fixed configuration card on the left and
// the pantry board / generated menu on the right. The planner and AI work run
// outside the view; this page only reads the resulting state.

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
	"github.com/gij072858-cmd/foodbox-go/internal/services/sichu"
)

// sichuPreset is one one-tap recipe for the current pantry.
type sichuPreset struct {
	Key   string
	Label string
}

var sichuPresets = []sichuPreset{
	{Key: "fridge", Label: "清冰箱临期特攻"},
	{Key: "solo", Label: "快手一人食"},
	{Key: "duo", Label: "两人两菜一汤"},
}

func (a *App) sichuPage(c *ui.Context, p design.Palette) {
	a.ensureSichuDashboard()
	a.pageBody(c, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Stretch).Gap(design.Sp(c, 4)).Children(func() {
			a.sichuConfigCard(c, p)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				switch {
				case a.sichu.generating && !a.sichu.planReady:
					a.sichuGeneratingCard(c, p)
				case a.sichu.planReady && !a.sichu.showBoard:
					a.sichuResultCard(c, p)
				default:
					a.sichuDashboardCard(c, p)
				}
			})
		})
	})
}

// ---- Left: configuration ------------------------------------------------

func (a *App) sichuConfigCard(c *ui.Context, p design.Palette) {
	ui.Column(c).Width(360).Shrink(0).MinWidth(0).Children(func() {
		design.CardPad(c, p).Grow(1).Children(func() {
			a.sichuEngineHeader(c, p)

			design.FieldLabel(c, p, "餐段")
			ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
				for _, o := range sichu.MealSlotOptions {
					o := o
					on := a.sichu.meals[o.Name]
					chip := design.Chip(c, p, "meal-"+o.Name, o.Label, on)
					if chip.Changed() {
						a.sichu.meals[o.Name] = !on
						a.clearSichuLocks()
					}
				}
			})

			design.FieldLabel(c, p, "谁吃这一餐")
			ui.Row(c).Gap(design.Sp(c, 2)).AlignItems(ui.Center).Wrap().Children(func() {
				a.sichuWhoControl(c, p)
				ui.Link(c, "管理成员", "/sichu/members").FontSize(design.FontMeta)
				ui.Link(c, "去完善", "/sichu/body").FontSize(design.FontMeta)
			})

			ui.Row(c).FillWidth().Gap(design.Sp(c, 2)).AlignItems(ui.Center).Wrap().Children(func() {
				if a.sichu.whoID == 0 {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "人数").FontSize(design.FontSmall).TextColor(p.Muted)
						servings := a.sichuServings()
						if _, changed := design.Stepper(c, p, "sichu-servings",
							&servings, 1, 12, "人", 110); changed {
							a.sichu.servings = float64(servings)
							a.clearSichuLocks()
						}
					})
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "临时客").FontSize(design.FontSmall).TextColor(p.Muted)
					if _, changed := design.Stepper(c, p, "sichu-guests",
						&a.sichu.guests, 0, 8, "人", 110); changed {
						a.clearSichuLocks()
					}
				})
			})

			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Text(c, a.sichuSummaryLine()).FontSize(design.FontMeta).
					TextColor(p.Faint).SingleLine()
				for _, line := range a.sichuLimitBadges() {
					if strings.HasPrefix(line, "口味 ") {
						continue
					}
					ui.Badge(c, line).FontSize(design.FontMeta).Padding(3, 8).
						Radius(design.RadiusSmall).Background(p.SurfaceAlt).TextColor(p.Muted)
				}
			})

			design.FieldLabel(c, p, "配餐模式")
			if design.Segmented(c, &a.sichu.style,
				"按人数智能配比", "一人食简餐", "自由指定菜数").Changed() {
				if a.sichu.style == 0 {
					counts := smartSichuCounts(a.sichuEffectiveServings())
					a.sichu.mains, a.sichu.vegs, a.sichu.soups =
						counts.Mains, counts.Vegs, counts.Soups
				}
				a.clearSichuLocks()
			}
			if a.sichu.style == 2 {
				ui.Row(c).Gap(design.Sp(c, 2)).Wrap().Children(func() {
					counts := a.sichuCounts()
					a.sichuCountControl(c, p, "荤菜", &a.sichu.mains, counts.Mains)
					a.sichuCountControl(c, p, "素菜", &a.sichu.vegs, counts.Vegs)
					a.sichuCountControl(c, p, "汤羹", &a.sichu.soups, counts.Soups)
				})
			} else {
				ui.Row(c).AlignSelf(ui.Start).Children(func() {
					design.Pill(c, p, a.sichuStructureLabel())
				})
			}

			if !a.pantryOff {
				design.FieldLabel(c, p, "库存策略")
				design.Segmented(c, &a.sichu.stockPolicy, "严守库存", "临期优先", "允许微缺")
			}
			ui.Checkbox(c, &a.sichu.avoidRecent3d, "避开近 3 天食历已吃菜式")

			ui.Box(c).Grow(1)
			if a.sichu.planError != "" {
				ui.Text(c, a.sichu.planError).FontSize(design.FontSmall).
					TextColor(p.Warning).Wrap()
			}
			if a.sichu.engineNotice != "" {
				ui.Text(c, a.sichu.engineNotice).FontSize(design.FontMeta).
					TextColor(p.Muted).Wrap()
			}
			if a.sichu.generating {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					loading.Arc(c, loading.Options{Size: 16, Color: p.Accent})
					ui.Text(c, "正在配餐…").FontSize(design.FontSmall).TextColor(p.Muted)
				})
			} else if ui.PrimaryButton(c, "生成配餐").FillWidth().Clicked() {
				a.generateSichuPlan(false)
			}
		})
	})
}

func (a *App) sichuEngineHeader(c *ui.Context, p design.Palette) {
	if !a.ai.configured && a.sichu.engine == sichuEngineAI {
		a.sichu.engine = sichuEngineLocal
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(design.Sp(c, 2)).Children(func() {
		ui.Text(c, "私厨配餐").FontSize(design.FontHeading).FontWeight(700).
			TextColor(p.Text).Grow(1).MinWidth(0).SingleLine()
		// design.Segmented pins itself to Start; a Column parent keeps that
		// from un-centering it inside this Row.
		ui.Column(c).Shrink(0).AlignItems(ui.End).Children(func() {
			disabled := []bool{false, !a.ai.configured}
			if design.SegmentedDisabled(c, &a.sichu.engine, disabled,
				"本地策略", "AI").Changed() {
				a.sichu.engineNotice = ""
			}
			ui.Link(c, "AI 设置", "/settings/ai").FontSize(design.FontMeta)
		})
	})
}

// sichuStructureLabel is the one-line readout for the modes that do not let
// the diner type counts.
func (a *App) sichuStructureLabel() string {
	if a.sichu.style == 1 {
		return "一碗端：1 主食 + 可选汤"
	}
	counts := smartSichuCounts(a.sichuEffectiveServings())
	parts := make([]string, 0, 3)
	parts = append(parts, fmt.Sprintf("%d荤", counts.Mains))
	parts = append(parts, fmt.Sprintf("%d素", counts.Vegs))
	if counts.Soups > 0 {
		parts = append(parts, fmt.Sprintf("%d汤", counts.Soups))
	}
	return "自动推荐：" + strings.Join(parts, " ")
}

// sichuSummaryLine is the compact read-only badge line: servings, body
// reference and taste. Long explanations stay out of the card.
func (a *App) sichuSummaryLine() string {
	parts := []string{fmt.Sprintf("%d 人份", a.sichuEffectiveServings())}
	parts = append(parts, a.sichuBasisLine())
	if taste := a.sichuTasteLine(); taste != "" {
		parts = append(parts, taste)
	}
	return strings.Join(parts, " · ")
}

func (a *App) sichuTasteLine() string {
	q := a.sichu.questionnaire
	parts := make([]string, 0, 2)
	if q.Spicy != "" && q.Spicy != "none" {
		parts = append(parts, sichu.LabelOf(sichu.SpicyOptions, q.Spicy))
	}
	if q.Saltiness == "light" {
		parts = append(parts, "清淡")
	}
	return strings.Join(parts, " · ")
}

// ---- Right: pantry board ------------------------------------------------

func (a *App) sichuDashboardCard(c *ui.Context, p design.Palette) {
	design.CardPad(c, p).FillWidth().Children(func() {
		design.SectionHeader(c, p, "今日看板", "")
		ui.Row(c).FillWidth().Gap(design.Sp(c, 2)).AlignItems(ui.Start).Wrap().Children(func() {
			if !a.pantryOff {
				a.sichuStatBlock(c, p, fmt.Sprintf("%d", a.sichu.dash.FullStocked),
					"道菜完全齐备", p.Success)
				a.sichuStatBlock(c, p, fmt.Sprintf("%d", len(a.sichu.dash.Expiring)),
					"样待消耗临期", p.Warning)
			}
			a.sichuStatBlock(c, p, a.sichu.dash.NutritionVerdict,
				"营养找平", p.Accent)
		})

		if names := a.sichu.dash.ExpiringNames; !a.pantryOff && len(names) > 0 {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
				shown := names
				if len(shown) > 4 {
					shown = shown[:4]
				}
				for _, name := range shown {
					name := name
					selected := a.sichu.stockPolicy == sichuStockExpiring
					chip := design.Chip(c, p, "expire-"+name, name, selected)
					if chip.Changed() {
						a.sichu.stockPolicy = sichuStockExpiring
					}
				}
				if len(names) > 4 {
					design.Pill(c, p, fmt.Sprintf("+%d", len(names)-4))
				}
			})
		}
		ui.Text(c, a.sichu.dash.NutritionHint).FontSize(design.FontSmall).
			TextColor(p.Muted).Wrap()

		ui.Divider(c)
		design.FieldLabel(c, p, "快速开始")
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
			for _, preset := range sichuPresets {
				if a.pantryOff && preset.Key == "fridge" {
					continue
				}
				preset := preset
				chip := design.Chip(c, p, "preset-"+preset.Key, preset.Label, false)
				if chip.Changed() {
					a.applySichuPreset(preset.Key)
				}
			}
		})
	})
}

func (a *App) sichuStatBlock(c *ui.Context, p design.Palette, value, label string, color ui.Color) {
	ui.Column(c).Grow(1).MinWidth(96).Gap(2).Children(func() {
		ui.Text(c, value).FontSize(22).FontWeight(700).TextColor(color).SingleLine()
		ui.Text(c, label).FontSize(design.FontSmall).TextColor(p.Muted).SingleLine()
	})
}

// applySichuPreset fills the left card and starts a plan in one tap.
func (a *App) applySichuPreset(key string) {
	if a.sichu.generating {
		return
	}
	switch key {
	case "fridge":
		a.sichu.style = 0
		a.sichu.stockPolicy = sichuStockExpiring
		if a.pantryOff {
			a.sichu.stockPolicy = sichuStockAllow
		}
		counts := smartSichuCounts(a.sichuEffectiveServings())
		a.sichu.mains, a.sichu.vegs, a.sichu.soups = counts.Mains, counts.Vegs, counts.Soups
	case "solo":
		a.sichu.style = 1
		a.sichu.servings = 1
		a.sichu.guests = 0
		a.sichu.stockPolicy = sichuStockAllow
	case "duo":
		a.sichu.style = 0
		a.sichu.servings = 2
		a.sichu.guests = 0
		a.sichu.stockPolicy = sichuStockAllow
		counts := smartSichuCounts(a.sichuEffectiveServings())
		a.sichu.mains, a.sichu.vegs, a.sichu.soups = counts.Mains, counts.Vegs, counts.Soups
	default:
		return
	}
	a.clearSichuLocks()
	a.sichu.planError = ""
	a.generateSichuPlan(false)
}

func (a *App) sichuGeneratingCard(c *ui.Context, p design.Palette) {
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
		for i := 0; i < 3; i++ {
			card := ui.Column(c).Grow(1).Padding(design.Sp(c, 3.5)).Gap(design.Sp(c, 2.5)).
				Radius(design.RadiusSmall).Border(1, p.Border)
			card.Children(func() {
				design.SkeletonFill(c, p, 86, design.RadiusSmall)
				design.Skeleton(c, p, 120, 13, 6)
				design.Skeleton(c, p, 76, 10, 5)
			})
		}
	})
}

// ---- Right: generated plan ---------------------------------------------

func (a *App) sichuResultCard(c *ui.Context, p design.Palette) {
	design.CardPad(c, p).FillWidth().Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			ui.Text(c, "配餐方案").FontSize(design.FontHeading).FontWeight(700).
				TextColor(p.Text).Grow(1).MinWidth(0).SingleLine()
			if a.sichu.planSource != "" {
				color := p.Muted
				if a.sichu.planSource == "AI" {
					color = p.Accent
				}
				design.Tag(c, p, a.sichu.planSource, color)
			}
			design.Pill(c, p, fmt.Sprintf("%d 人份", a.sichu.planServings))
			if a.sichu.generating {
				loading.Arc(c, loading.Options{Size: 14, Color: p.Accent})
			}
			if design.Button(c, p, "sichu-board", "看板", design.ButtonGhost).Clicked() {
				a.sichu.showBoard = true
			}
			if design.Button(c, p, "sichu-again", "换一批", design.ButtonSecondary).Clicked() {
				a.generateSichuPlan(true)
			}
		})
		if a.sichu.planSummary != "" {
			ui.Text(c, a.sichu.planSummary).FontSize(design.FontSmall).
				TextColor(p.Muted).SingleLine()
		}

		for mi, meal := range a.sichu.plan.Meals {
			mi, meal := mi, meal
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, meal.Title+" · "+a.sichuPlanStyleLabel()).
					FontSize(design.FontSubhead).FontWeight(600).TextColor(p.Text).
					Margin(0, 0, 2, 0)
				for di, dish := range meal.Dishes {
					di, dish := di, dish
					a.sichuWorkbenchDishRow(c, p, mi, di, dish)
				}
			})
		}

		a.sichuNutritionLine(c, p)
		ui.Divider(c)
		a.sichuResultActions(c, p)
	})
}

// sichuWorkbenchDishRow is two lines on purpose: the right column is only
// about 352 wide at the 1024 minimum window.
func (a *App) sichuWorkbenchDishRow(c *ui.Context, p design.Palette, mealIndex, dishIndex int, dish sichu.Dish) {
	ui.Column(c).FillWidth().Padding(design.Sp(c, 2.5)).Gap(4).
		Radius(design.RadiusSmall).Background(p.Surface).Border(1, p.Border).
		Role(ui.RoleListItem).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			design.Tag(c, p, dishSlotLabel(dish.Slot), p.Accent)
			ui.Text(c, dish.Name).FontSize(design.FontBody).FontWeight(600).
				TextColor(p.Text).Grow(1).MinWidth(0).SingleLine()
			if !a.pantryOff {
				a.sichuStockBadge(c, p, dish)
			}
		})
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Wrap().Children(func() {
			meta := make([]string, 0, 3)
			if dish.Brief != "" {
				meta = append(meta, dish.Brief)
			}
			if dish.DurationMinutes > 0 {
				meta = append(meta, fmt.Sprintf("%d 分钟", dish.DurationMinutes))
			}
			if a.sichuLocked(mealIndex, dishIndex) {
				meta = append(meta, "已锁定")
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(design.FontMeta).
				TextColor(p.Muted).Grow(1).MinWidth(0).SingleLine()

			lockLabel := "锁定"
			if a.sichuLocked(mealIndex, dishIndex) {
				lockLabel = "已锁"
			}
			if design.Button(c, p, fmt.Sprintf("sichu-lock-%d-%d", mealIndex, dishIndex),
				lockLabel, design.ButtonGhost).
				Tooltip("锁定此菜：换一批时保留").Clicked() {
				a.toggleSichuLock(mealIndex, dishIndex, dish)
			}
			if design.Button(c, p, fmt.Sprintf("sichu-reroll-%d-%d", mealIndex, dishIndex),
				"重抽", design.ButtonGhost).
				Tooltip("单菜重抽：只换这一道").Clicked() {
				a.replaceSichuDish(mealIndex, dishIndex)
			}
		})
		if dish.ServeNote != "" {
			ui.Text(c, "分餐："+dish.ServeNote).FontSize(design.FontMeta).
				TextColor(p.Faint).Wrap()
		}
	})
}

// sichuStockBadge reads the app-wide match cache; it never matches in the
// view.
func (a *App) sichuStockBadge(c *ui.Context, p design.Palette, dish sichu.Dish) {
	if dish.IsDraft() {
		design.Tag(c, p, "新菜草案", p.Warning)
		return
	}
	if len(a.items) == 0 {
		design.Tag(c, p, "库存未记录", p.Muted)
		return
	}
	res, ok := a.recipeMatches[dish.RecipeID]
	if !ok {
		design.Tag(c, p, "库存未知", p.Muted)
		return
	}
	if res.Status == cookmatch.CanCook {
		design.Tag(c, p, "齐备", p.Success)
		return
	}
	label := fmt.Sprintf("差 %d 样", len(res.Missing))
	if len(res.Missing) > 0 {
		label += "：" + res.Missing[0].Name
		if len(res.Missing) > 1 {
			label += "等"
		}
	}
	design.Tag(c, p, label, p.Warning)
}

func (a *App) sichuNutritionLine(c *ui.Context, p design.Palette) {
	if a.sichu.planKcal == 0 && a.sichu.planProtein == 0 {
		ui.Text(c, "营养数据不足，暂不给数字。").FontSize(design.FontMeta).
			TextColor(p.Faint).SingleLine()
		return
	}
	ui.Text(c, fmt.Sprintf("约 %d kcal · 蛋白 %d g（估算）", a.sichu.planKcal, a.sichu.planProtein)).
		FontSize(design.FontMeta).TextColor(p.Faint).SingleLine()
}

// sichuPlanStyleLabel names the mode that produced the plan, not the service
// style (smart mode is sent to the planner as a custom count).
func (a *App) sichuPlanStyleLabel() string {
	switch a.sichu.planStyle {
	case 1:
		return "一碗端"
	case 2:
		return "自由指定"
	default:
		return "智能配比"
	}
}

func (a *App) sichuResultActions(c *ui.Context, p design.Palette) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
		ui.Icon(c, &icons.Clock).Size(14, 14).TextColor(p.Ink)
		text := "预计耗时以掌序排程为准"
		if a.sichu.planMinutes > 0 {
			text = fmt.Sprintf("预计掌序并行耗时约 %d 分钟", a.sichu.planMinutes)
		}
		ui.Text(c, text).FontSize(design.FontSmall).TextColor(p.Muted).
			Grow(1).MinWidth(140).SingleLine()
		if a.sichu.pushed {
			if ui.PrimaryButton(c, "已推入掌序 · 去掌序").Clicked() {
				a.router.Push("/schedule")
			}
		} else if ui.PrimaryButton(c, "推入「掌序」开做").Clicked() {
			if a.addPlanToSchedule() {
				a.router.Push("/schedule")
			}
		}
	})
	if a.sichu.resultNotice != "" {
		ui.Text(c, a.sichu.resultNotice).FontSize(design.FontMeta).
			TextColor(p.Accent).Wrap()
	}
}

// ---- Shared controls ----------------------------------------------------

// sichuServings is the serving count as a whole number, at least one.
func (a *App) sichuServings() int {
	n := int(a.sichu.servings + 0.5)
	if n < 1 {
		return 1
	}
	return n
}

// sichuCounts is what the counters show: the smart structure in the smart
// mode, the diner's own numbers otherwise.
func (a *App) sichuCounts() sichu.Counts {
	if a.sichu.style == 0 {
		return smartSichuCounts(a.sichuEffectiveServings())
	}
	return sichu.Counts{Mains: a.sichu.mains, Vegs: a.sichu.vegs, Soups: a.sichu.soups}
}

// sichuCountControl is one dish counter. It only appears in the free mode.
func (a *App) sichuCountControl(c *ui.Context, p design.Palette, label string, field *int, shown int) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(design.FontSmall).TextColor(p.Muted)
		value := shown
		if _, changed := design.Stepper(c, p, "sichu-count-"+label, &value,
			0, 6, "道", 96); changed {
			*field = value
			a.clearSichuLocks()
		}
	})
}

// sichuWhoControl is the diner chooser: a segmented control while the
// household fits in one row, a menu once it does not. whoID stays the one
// source of truth either way.
func (a *App) sichuWhoControl(c *ui.Context, p design.Palette) {
	ids := []int64{0}
	names := []string{"本人"}
	for _, m := range a.sichu.members {
		if m.IsSelf {
			continue
		}
		ids = append(ids, m.ID)
		names = append(names, m.Name)
	}
	idx := 0
	for i, id := range ids {
		if id == a.sichu.whoID {
			idx = i
		}
	}
	if len(names) <= 4 {
		// The caller is a horizontal Row; a Column keeps AlignSelf(Start)
		// from reading as "top-aligned".
		ui.Column(c).Shrink(0).Children(func() {
			if design.Segmented(c, &idx, names...).Changed() {
				a.setSichuWho(ids[idx])
			}
		})
		return
	}
	current := names[idx]
	if ui.Select(c, &current, names).Changed() {
		for i, name := range names {
			if name == current {
				a.setSichuWho(ids[i])
				break
			}
		}
	}
}

// setSichuWho records the diner and keeps servings and limits in step.
func (a *App) setSichuWho(id int64) {
	a.sichu.whoID = id
	if id != 0 {
		a.sichu.servings = 1
	}
	a.clearSichuLocks()
	a.refreshSichuLimits()
}

// sichuBasisLine says, in one line, what the plan can lean on.
func (a *App) sichuBasisLine() string {
	pr := a.sichu.profile
	if pr.HeightCm <= 0 && pr.WeightKg <= 0 {
		return "身体档案未填"
	}
	parts := make([]string, 0, 2)
	if pr.HeightCm > 0 {
		parts = append(parts, fmt.Sprintf("%d cm", pr.HeightCm))
	}
	if pr.WeightKg > 0 {
		parts = append(parts, fmt.Sprintf("%d kg", pr.WeightKg))
	}
	return strings.Join(parts, " / ")
}

// sichuLimitBadges shows at most four limits, the rest as +N: one line,
// never a wall of tags.
func (a *App) sichuLimitBadges() []string {
	labels := a.sichu.limitLabels
	if len(labels) == 0 {
		return []string{"无特别限制"}
	}
	if len(labels) <= 4 {
		return labels
	}
	out := append([]string{}, labels[:4]...)
	return append(out, fmt.Sprintf("+%d", len(labels)-4))
}

func sichuCmOrDash(v int) string {
	if v <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d cm", v)
}

func sichuKgOrDash(v int) string {
	if v <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d kg", v)
}

func sichuAgeOrDash(v int) string {
	if v <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d 岁", v)
}

func sichuActivityOrDash(level string) string {
	if strings.TrimSpace(level) == "" {
		return "—"
	}
	return sichu.LabelOf(sichu.ActivityOptions, level)
}
