package app

// The detail page's hero is the dish's face, and it carries information
// instead of pretending to be a photo: the macro split of one serving as a
// ring, what the pantry is missing, the purine reading, and how long the
// dish takes. Every number comes from the dish cache; the card draws no
// animation of its own, so a settled page still asks for no frames.

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/nutrition"
)

const (
	heroWidth  = 320
	heroHeight = 220
)

// recipeHeroCard is the 320×220 block at the head of the detail page.
func (a *App) recipeHeroCard(c *ui.Context, p design.Palette, rec domain.Recipe) {
	nut, have := a.dishNut(rec)
	card := ui.Column(c).Width(heroWidth).Height(heroHeight).Shrink(0).
		Radius(design.RadiusCard).Background(p.Surface).Border(1, p.Border).
		Padding(design.Sp(c, 4)).Gap(design.Sp(c, 2))
	design.RestShadow(card, p)
	// The hero stays flat. Its surface is text and arcs, and tilting a
	// card of them smears the type; the mark tile inside it is the piece of
	// artwork the dish page shares with the list.
	card.Children(func() {
		ui.Row(c).FillWidth().Gap(design.Sp(c, 2)).AlignItems(ui.Center).Children(func() {
			a.recipeMarkTile(c, p, rec, 26)
			ui.Text(c, "营养估算 · 每 1 人份").FontSize(design.FontMeta).TextColor(p.Muted).Grow(1)
			ui.Text(c, coverageLabel(nut, have)).FontSize(design.FontCaption).
				TextColor(coverageColor(p, nut, have)).SingleLine()
		})
		body := ui.Row(c).FillWidth().Gap(design.Sp(c, 3)).AlignItems(ui.Center)
		if have {
			body.Children(func() {
				a.macroSummary(c, p, nut.Nutrients)
				a.macroLegend(c, p, nut.Nutrients)
			})
		} else {
			body.Justify(ui.Center).Children(func() {
				ui.Text(c, "营养目录读取中").FontSize(design.FontSmall).TextColor(p.Faint)
			})
		}
		ui.Divider(c)
		a.heroFooter(c, p, rec, nut, have)
	})
}

// macroSummary is the dish's energy in one number. The split it used to be
// drawn as a ring of is now three meters in the legend, where it can be
// read without a key.
func (a *App) macroSummary(c *ui.Context, p design.Palette, n nutrition.Nutrients) {
	ui.Column(c).Width(84).Shrink(0).Gap(0).Justify(ui.Center).Children(func() {
		ui.Text(c, kcalLabel(n)).FontSize(design.FontTitle).FontWeight(700).
			TextColor(p.Text).TextAlign(ui.Center)
		ui.Text(c, "kcal").FontSize(design.FontCaption).TextColor(p.Faint).TextAlign(ui.Center)
	})
}

func (a *App) macroLegend(c *ui.Context, p design.Palette, n nutrition.Nutrients) {
	prot, fat, carb, total := macroEnergy(n)
	col := ui.Column(c).Grow(1).MinWidth(0).Gap(design.Sp(c, 1.5)).Justify(ui.Center)
	col.Children(func() {
		a.macroRow(c, p, "蛋白", n.ProteinG, share(prot, total), p.Success)
		a.macroRow(c, p, "脂肪", n.FatG, share(fat, total), p.Warning)
		a.macroRow(c, p, "碳水", n.CarbG, share(carb, total), p.Accent)
	})
}

func (a *App) macroRow(c *ui.Context, p design.Palette, label string, grams float64, frac float32, color ui.Color) {
	ui.Column(c).FillWidth().Gap(design.Sp(c, 0.75)).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(6).Children(func() {
			// Meter colours sit at Success whatever the share is, so the
			// dot keeps the three macros apart.
			ui.Box(c).Size(6, 6).Radius(design.RadiusPill).Background(color)
			ui.Text(c, label).FontSize(design.FontMeta).TextColor(p.Muted).Grow(1)
			ui.Text(c, fmt.Sprintf("%.0f g · %.0f%%", grams, frac*100)).
				FontSize(design.FontMeta).FontWeight(600).TextColor(p.Text)
		})
		ui.Meter(c, float64(frac), 0, 1, nil).Label(label + " 占一餐能量")
	})
}

// heroFooter keeps the two readings that need the whole card's width and
// have no place else: what the pantry is missing, and the purine line.
// Difficulty, steps and season stay as pills beside the title.
func (a *App) heroFooter(c *ui.Context, p design.Palette, rec domain.Recipe, nut dishNutrition, have bool) {
	ui.Row(c).FillWidth().Gap(design.Sp(c, 3)).AlignItems(ui.Start).Children(func() {
		if !a.pantryOff {
			a.heroStockMini(c, p, rec)
		}
		heroMini(c, p, 1, "嘌呤", purineLabel(nut, have), purineColor(p, nut, have), nil)
	})
}

// heroStockMini closes the card with the one question a home cook asks
// before the numbers: can I make this now.
func (a *App) heroStockMini(c *ui.Context, p design.Palette, rec domain.Recipe) {
	res := a.recipeMatches[rec.ID]
	required, optional := 0, 0
	for _, in := range rec.Ingredients {
		if in.Required {
			required++
		} else {
			optional++
		}
	}
	got := required - len(res.Missing)
	if got < 0 {
		got = 0
	}
	color := p.Success
	if got < required {
		color = p.Warning
	}
	level := func() {
		if required == 0 {
			return
		}
		ui.Meter(c, float64(got), 0, float64(required),
			&ui.MeterLevels{Critical: 0.5 * float64(required), Warning: 0.99 * float64(required)}).
			Label("必需食材在库")
	}
	_ = optional
	heroMini(c, p, 1.5, "必需食材", fmt.Sprintf("%d / %d 在库", got, required), color, level)
}

func heroMini(c *ui.Context, p design.Palette, grow float32, label, value string, color ui.Color, extra func()) {
	col := ui.Column(c).Grow(grow).MinWidth(0).Gap(design.Sp(c, 0.5))
	col.Children(func() {
		ui.Text(c, label).FontSize(design.FontCaption).TextColor(p.Faint).SingleLine()
		ui.Text(c, value).FontSize(design.FontSmall).FontWeight(600).TextColor(color).SingleLine()
		if extra != nil {
			extra()
		}
	})
}

// macroEnergy turns grams into the 4/9/4 kilocalorie split the ring and the
// legend both read, so the two can never disagree.
func macroEnergy(n nutrition.Nutrients) (prot, fat, carb, total float32) {
	prot = float32(n.ProteinG) * 4
	fat = float32(n.FatG) * 9
	carb = float32(n.CarbG) * 4
	return prot, fat, carb, prot + fat + carb
}

func share(part, total float32) float32 {
	if total <= 0 {
		return 0
	}
	return part / total
}

func kcalLabel(n nutrition.Nutrients) string {
	if n.Kcal <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f", n.Kcal)
}

func coverageLabel(nut dishNutrition, have bool) string {
	switch {
	case !have:
		return "目录未就绪"
	case nut.Nutrients.Coverage == nutrition.CoverageFull:
		return "食材表全覆盖"
	case nut.Nutrients.Coverage == nutrition.CoveragePartial:
		return "部分为估算"
	default:
		return "食材表未覆盖"
	}
}

func coverageColor(p design.Palette, nut dishNutrition, have bool) ui.Color {
	if !have {
		return p.Faint
	}
	switch nut.Nutrients.Coverage {
	case nutrition.CoverageFull:
		return p.Success
	case nutrition.CoveragePartial:
		return p.Warning
	default:
		return p.Faint
	}
}

// purineLabel shows the weighed estimate when there is one and the tier of
// the worst ingredient always: a dish can read 0 mg per serving and still
// carry a high-purine ingredient, which is exactly what the gout-aware user
// needs to see.
func purineLabel(nut dishNutrition, have bool) string {
	tier := string(nut.Purine)
	if !have {
		return "—"
	}
	if tier == "" {
		tier = string(nutrition.PurineUnknown)
	}
	if nut.Nutrients.PurineMg > 0 {
		return fmt.Sprintf("%.0f mg · %s", nut.Nutrients.PurineMg, tier)
	}
	return "未测 mg · " + tier
}

func purineColor(p design.Palette, nut dishNutrition, have bool) ui.Color {
	if !have {
		return p.Faint
	}
	switch nut.Purine {
	case nutrition.PurineHigh:
		return p.Danger
	case nutrition.PurineMedium:
		return p.Warning
	case nutrition.PurineLow:
		return p.Success
	}
	return p.Faint
}
