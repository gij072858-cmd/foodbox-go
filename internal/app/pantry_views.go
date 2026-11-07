package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

func (a *App) pantryPage(c *ui.Context, p design.Palette) {
	now := time.Now().Unix()
	depleted, eatFirstCount := 0, 0
	eatFirst := make([]domain.Item, 0, 6)
	for _, it := range a.items {
		if it.IsDepleted {
			depleted++
			continue
		}
		if it.NeedsEatingFirst(now) {
			eatFirstCount++
			if len(eatFirst) < 3 {
				eatFirst = append(eatFirst, it)
			}
		}
	}
	inStock := len(a.items) - depleted

	ui.Column(c).Grow(1).FillWidth().MinWidth(0).Children(func() {
		if len(a.items) == 0 {
			a.pageBody(c, func() {
				a.emptyPantry(c, p)
			})
			return
		}
		a.pageFrame(c, func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 2.5)).Padding(0, 0, design.Sp(c, 2), 0).Children(func() {
				a.pantryGreeting(c, p, inStock, eatFirstCount)
				a.pantryFocusBanner(c, p, eatFirst, now)
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					if ui.SearchField(c, &a.pantryQuery).Width(220).
						Placeholder("搜索仓库").Label("搜索仓库").Changed() {
						a.recomputePantryView()
					}
					idx := pantryFilterIndex(a.pantryFilter)
					labels := []string{
						fmt.Sprintf("全部(%d)", len(a.items)),
						fmt.Sprintf("在用(%d)", len(a.items)-depleted),
						fmt.Sprintf("临期(%d)", eatFirstCount),
						fmt.Sprintf("用完(%d)", depleted),
					}
					if design.Segmented(c, &idx, labels...).Changed() {
						a.pantryFilter = pantryFilterNames[idx]
						a.recomputePantryView()
					}
					ui.Box(c).Grow(1)
				})
			})

			if len(a.pantryView) == 0 {
				empty := design.EmptyState(c, p, &icons.Pantry, "没有匹配的条目",
					"换个关键词，或点「全部」看整个仓库。")
				empty.Margin(design.PagePadding, 0, 0, 0)
				empty.Children(func() {
					if design.Button(c, p, "pantry-clear", "清除筛选", design.ButtonSecondary).Clicked() {
						a.pantryQuery = ""
						a.pantryFilter = ""
						a.recomputePantryView()
					}
				})
				return
			}

			// The framework's grid picks the column count from the room —
			// four at 1280 — so a narrow window reflows without a second
			// layout rule and keeps its scroll position. 112 keeps five
			// rows (eighteen items) on the first screen under the new
			// greeting and banner.
			ui.GridView(c.Key("pantry-grid"), &a.pantryGrid, len(a.pantryView),
				220, 112, func(i int) {
					card := a.pantryCard(c, p, a.pantryView[i], now, 112)
					design.StaggerRow(card, design.Stagger(c, a.level, a.pageAt, i))
				}).Grow(1).FillWidth().MinWidth(0).
				Padding(design.PagePadding, 0, design.PagePadding, 0)
		})
	})

	a.itemDetailModal(c, p)
	a.quickEditModal(c, p)
	a.customItemModal(c, p)
}

// pantryGreeting is the home's first breath: the day and the stock in one
// quiet line, then the hour's greeting in the page's largest type.
func (a *App) pantryGreeting(c *ui.Context, p design.Palette, inStock, eatFirst int) {
	now := time.Now()
	weekday := [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[int(now.Weekday())]
	line := fmt.Sprintf("%d月%d日 %s · 在库 %d 样", int(now.Month()), now.Day(), weekday, inStock)
	if eatFirst > 0 {
		line += fmt.Sprintf(" · %d 样临期", eatFirst)
	}
	ui.Column(c).FillWidth().Gap(2).Padding(design.Sp(c, 1), 0, design.Sp(c, 1), 0).Children(func() {
		ui.Text(c, line).FontSize(design.FontSmall).TextColor(p.Muted).SingleLine()
		ui.Text(c, dayGreeting(now)+"，今天想吃什么？").FontSize(design.FontHero).FontWeight(700).
			TextColor(p.Text).SingleLine()
	})
}

// dayGreeting names the part of the day the way a person would.
func dayGreeting(t time.Time) string {
	switch h := t.Hour(); {
	case h < 5:
		return "夜深了"
	case h < 11:
		return "早上好"
	case h < 14:
		return "中午好"
	case h < 18:
		return "下午好"
	default:
		return "晚上好"
	}
}

// pantryFocusBanner is the one card above the grid: what to eat first on
// the left, the four household errands on the right. It replaces the row of
// loose pills that used to scatter across the page's top.
func (a *App) pantryFocusBanner(c *ui.Context, p design.Palette, eatFirst []domain.Item, now int64) {
	banner := ui.Row(c).FillWidth().Padding(design.Sp(c, 3)).Gap(design.Sp(c, 4)).
		AlignItems(ui.Center).Radius(design.RadiusCard).Border(1, p.Border)
	design.RestShadow(banner, p)
	banner.Draw(func(pnt *ui.Painter, r ui.Rect) {
		pnt.FillGradient(r, ui.LinearGradient{Angle: 100, Stops: []ui.GradientStop{
			{Color: p.Warning.Alpha(0.10), At: 0},
			{Color: p.AccentPale, At: 1},
		}}, design.RadiusCard)
	})
	banner.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(design.Sp(c, 1.5)).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, &icons.Alert).Size(14, 14).TextColor(p.Warning)
				label := "先吃我 · 临期提醒"
				if len(eatFirst) == 0 {
					label = "先吃我 · 库存状态良好"
				}
				ui.Text(c, label).FontSize(design.FontSmall).FontWeight(700).
					TextColor(p.Warning).SingleLine()
			})
			if len(eatFirst) > 0 {
				ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
					for _, it := range eatFirst {
						a.eatFirstChip(c, p, it, now)
					}
				})
			} else {
				ui.Text(c, "没有临期食材，按平常的节奏做就好。").
					FontSize(design.FontSmall).TextColor(p.Muted).SingleLine()
			}
			ui.Row(c).Gap(6).Children(func() {
				if design.Button(c, p, "pantry-eat-first", "去配一餐",
					design.ButtonPrimary).Clicked() {
					a.router.Push("/sichu")
				}
			})
		})
		ui.Column(c).Width(300).Shrink(0).Gap(6).AlignItems(ui.End).Children(func() {
			ui.Row(c).Gap(6).Wrap().Justify(ui.End).Children(func() {
				ui.PopoverMenuButton(c, "+ 添加食材", func(m *ui.Menu) {
					if m.Item("从系统目录添加").Chosen() {
						a.router.Push("/pantry/catalog")
					}
					if m.Item("自建条目").Chosen() {
						a.customOpen = true
					}
				})
			})
			ui.Row(c).Gap(6).Wrap().Justify(ui.End).Children(func() {
				if design.Button(c, p, "pantry-reconcile", "盘点核销",
					design.ButtonSecondary).Clicked() {
					a.router.Push("/pantry/reconcile")
				}
				if design.Button(c, p, "pantry-pack", "新手包",
					design.ButtonSecondary).Clicked() {
					a.router.Push("/pantry/starter")
				}
				if design.Button(c, p, "pantry-shopping",
					fmt.Sprintf("购物清单 %d", len(a.shopping)),
					design.ButtonSecondary).Clicked() {
					a.router.Push("/pantry/shopping")
				}
			})
		})
	})
}

// pantryFilterNames are the segmented control's choices, in order; the
// index is the control's state, the name is what recomputePantryView
// reads. The counts live in the labels, so the number is said once.
var pantryFilterNames = []string{"", "在用", "临期", "用完"}

func pantryFilterIndex(filter string) int {
	for i, name := range pantryFilterNames {
		if name == filter {
			return i
		}
	}
	return 0
}

func (a *App) emptyPantry(c *ui.Context, p design.Palette) {
	card := design.EmptyState(c, p, &icons.Pantry, "仓库还是空的",
		"从 1,000 条系统目录里挑几样，或导入一份地域新手包。")
	card.Children(func() {
		ui.Row(c).Gap(10).Margin(8, 0, 0, 0).Children(func() {
			if design.Button(c, p, "empty-catalog", "从目录添加", design.ButtonPrimary).Clicked() {
				a.router.Push("/pantry/catalog")
			}
			if design.Button(c, p, "empty-pack", "新手包", design.ButtonSecondary).Clicked() {
				a.router.Push("/pantry/starter")
			}
			if design.Button(c, p, "empty-custom", "自建条目", design.ButtonGhost).Clicked() {
				a.customOpen = true
			}
		})
	})
}

func (a *App) eatFirstChip(c *ui.Context, p design.Palette, it domain.Item, now int64) {
	chip := ui.ButtonBase(c.Key(fmt.Sprintf("eat-first-%d", it.ID))).Row().Gap(6).
		Padding(4, 10).Radius(design.RadiusPill).Label(it.Name)
	design.Wash(chip, p.Warning.Alpha(0.12), p.Warning.Alpha(0.20))
	design.Lift(chip, 0, 0.96)
	chip.Children(func() {
		ui.Text(c, it.Name).FontSize(design.FontMeta).FontWeight(600).TextColor(p.Text)
		ui.Text(c, shortExpiry(it, now)).FontSize(design.FontMeta).TextColor(p.Warning)
	})
	if chip.Clicked() {
		a.openItem(it.ID)
	}
}

func (a *App) pantryCard(c *ui.Context, p design.Palette, it domain.Item, now int64, height float32) ui.Element {
	stage := ui.Box(c).Grow(1).MinWidth(0).Height(height)
	stage.Children(func() {
		// Not clipped: a clip under a 3D transform is re-rasterized on the
		// CPU every frame of the lean (design/gravity.go). The corners are
		// rounded by the photo, the scrim and the fallback field instead.
		card := ui.ButtonBase(c.Key(fmt.Sprintf("pantry-%d", it.ID))).Column().FillWidth().
			Height(height).Radius(design.RadiusSmall).AlignItems(ui.Start).
			Border(1, p.Border).Label(it.Name)
		design.Wash(card, p.Surface, p.Hover)
		design.Lift(card, 2, 0.98)
		key := a.itemImageKey(it)
		if key != "" {
			a.images.Request(key, func() { a.post(func() {}) })
		}
		var glare ui.Element
		card.Children(func() {
			// The photo is the whole card, and it stays sharp: the left side
			// fades into the surface behind the words, the right side is the
			// ingredient itself. This is Cloudy's two-layer cover idea
			// (sharp art over a quiet field, ui_cover.go) applied to a card;
			// blurring the only picture a row has is what erased it.
			bg := ui.Box(c).FillWidth().Height(height).PassThrough()
			glare = bg
			bg.Children(func() {
				if bm := a.images.Get(key); bm != nil {
					ui.Image(c, bm).Absolute().Top(0).Left(0).Right(0).Bottom(0).
						Fit(ui.Cover).Radius(design.RadiusSmall)
				} else {
					ui.Box(c).Fill().Radius(design.RadiusSmall).Background(p.SurfaceAlt)
				}
				scrim := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0)
				scrim.Draw(func(pnt *ui.Painter, r ui.Rect) {
					pnt.FillGradient(r, ui.LinearGradient{Angle: 90, Oklab: true,
						Stops: []ui.GradientStop{
							{Color: p.Surface.Alpha(0.96), At: 0},
							{Color: p.Surface.Alpha(0.86), At: 0.30},
							{Color: p.Surface.Alpha(0.45), At: 0.55},
							{Color: p.Surface.Alpha(0.0), At: 0.78},
						}}, design.RadiusSmall)
				})
				if it.IsDepleted {
					bg.Opacity(0.55)
				}
			})
			content := ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).
				Padding(design.Sp(c, 2)).Gap(design.Sp(c, 0.75)).
				AlignItems(ui.Start).PassThrough()
			content.Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					chip := ui.Box(c).Size(22, 22).Radius(6).
						Background(p.Surface.Alpha(0.72)).Center()
					chip.Children(func() {
						ui.Icon(c, pantryIcon(it.Category)).Size(14, 14).TextColor(p.Ink)
					})
					name := ui.Text(c, it.Name).FontSize(design.FontSmall).FontWeight(600).
						TextColor(p.Text).Grow(1).SingleLine()
					if it.IsDepleted {
						name.TextColor(p.Faint)
					}
				})
				// Storage first, then how much is left, as a cook reads a shelf.
				meta := storageLabel(it.StorageLocation)
				if left := it.RemainingText(); left != "" {
					if meta != "" {
						meta += " · "
					}
					meta += left
				}
				ui.Text(c, meta).FontSize(design.FontMeta).TextColor(p.Muted).SingleLine()
				ui.Box(c).Grow(1)
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					// These two ride the photo's clearer side, so they carry
					// a surface pill instead of a translucent tint.
					kind := design.Tag(c, p, pantryKindLabel(it), p.Muted)
					kind.Background(p.Surface.Alpha(0.82))
					ui.Box(c).Grow(1)
					if label := expiryLabel(it, now); label != "" {
						color := p.Muted
						if it.NeedsEatingFirst(now) {
							color = p.Warning
						}
						tag := design.Tag(c, p, label, color)
						tag.Background(p.Surface.Alpha(0.82))
					}
				})
			})
		})
		// The stage reads the pointer and the card leans; the light lands on
		// the photo layer, under the text, so the words stay crisp. This is
		// the split design.Tilt documents: an element asking for the pointer
		// is interactive, so it must not be the one being leaned.
		design.Tilt(c, stage, card, glare, design.RadiusSmall, pantryGlarePeak(p), a.level.Full())
		if card.Clicked() {
			a.openItem(it.ID)
		}
	})
	return stage
}

// pantryGlarePeak keeps the light over a photographed card below Cloudy's
// 0.26: that value is for bare artwork, while this card puts white text over
// the same pixels. The dark appearance gets less light still.
func pantryGlarePeak(p design.Palette) float32 {
	if p.Dark {
		return 0.10
	}
	return 0.18
}

// pantryKindLabel is the one-word shelf class: fresh food, a staple, or a
// seasoning.
func pantryKindLabel(it domain.Item) string {
	switch {
	case it.Kind == domain.ItemKindSeasoning:
		return "调料"
	case domain.ZoneOf(it) == domain.ZoneStaple:
		return "底料"
	default:
		return "生鲜"
	}
}

// pantryIcon gives a card a category mark without drawing a second cover.
func pantryIcon(category string) *ui.SVG {
	switch category {
	case "蔬菜", "菌菇", "根茎":
		return &icons.Leaf
	case "猪肉", "牛肉", "羊肉", "鸡肉", "鸭肉", "禽肉":
		return &icons.Flame
	case "鱼类", "虾蟹贝", "其他海鲜":
		return &icons.Fish
	case "蛋类", "豆制品":
		return &icons.Egg
	case "主食", "面点":
		return &icons.Bowl
	}
	return &icons.Pantry
}

func (a *App) itemImageKey(it domain.Item) string {
	if it.ImagePath != "" {
		return it.ImagePath
	}
	if key, ok := assets.ItemImage(it.Name); ok {
		return key
	}
	if key, ok := assets.CategoryImage(it.Category); ok {
		return key
	}
	return ""
}

func expiryLabel(it domain.Item, now int64) string {
	if it.ExpireAt == nil || it.IsDepleted {
		return ""
	}
	days := (it.ExpireAt.Unix() - now) / 86400
	switch {
	case days < 0:
		return fmt.Sprintf("已过期 %d 天", -days)
	case days == 0:
		return "今天到期"
	case days <= 3:
		return fmt.Sprintf("还剩 %d 天", days)
	default:
		return it.ExpireAt.Format("01-02") + " 到期"
	}
}

func shortExpiry(it domain.Item, now int64) string {
	if it.ExpireAt == nil {
		if it.GranularLevel == domain.GranularLow {
			return "快没了"
		}
		return "先吃"
	}
	days := (it.ExpireAt.Unix() - now) / 86400
	if days < 0 {
		return "过期"
	}
	if days == 0 {
		return "今天"
	}
	return fmt.Sprintf("%d 天", days)
}

func (a *App) openItem(id int64) {
	if _, ok := a.itemsByID[id]; !ok {
		return
	}
	a.itemDetail = id
	a.detailOpen = true
}

func (a *App) itemDetailModal(c *ui.Context, p design.Palette) {
	if a.itemDetail == 0 {
		return
	}
	it, ok := a.itemsByID[a.itemDetail]
	if !ok {
		a.itemDetail = 0
		return
	}
	design.ModalDialog(c, p, &a.detailOpen, 540, a.level, func() {
		ui.Row(c).FillWidth().Gap(14).Children(func() {
			cover := ui.Box(c).Width(88).Height(88).Radius(12).Center()
			cover.Draw(func(pnt *ui.Painter, r ui.Rect) {
				pnt.FillGradient(r, ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
					{Color: p.CoverStart, At: 0}, {Color: p.CoverEnd, At: 1},
				}}, 12)
			})
			key := a.itemImageKey(it)
			if key != "" {
				a.images.Request(key, func() { a.post(func() {}) })
			}
			// The sheet's cover is pure art, so the art inside it leans and
			// catches the glare; the cover box is what the pointer is read
			// from, and nothing in the sheet needs the press it would take.
			var art ui.Element
			cover.Children(func() {
				art = ui.Box(c).Fill().Radius(12)
				art.Children(func() {
					if bm := a.images.Get(key); bm != nil {
						ui.Image(c, bm).Fill().Fit(ui.Cover).Radius(12)
					} else {
						ui.Text(c, firstRune(it.Name)).FontSize(34).FontWeight(700).
							TextColor(p.Accent.Alpha(0.3))
					}
				})
			})
			design.Tilt(c, cover, art, art, 12, design.GlarePeak, a.level.Full())
			ui.Column(c).Grow(1).Gap(4).Justify(ui.Center).Children(func() {
				ui.Text(c, it.Name).FontSize(24).FontWeight(700).TextColor(p.Text)
				ui.Text(c, fmt.Sprintf("%s · %s · %s", it.Category, zoneName(it), it.RemainingText())).
					FontSize(12).TextColor(p.Muted)
				if label := expiryLabel(it, time.Now().Unix()); label != "" {
					ui.Text(c, label).FontSize(12).TextColor(p.Warning)
				}
			})
			if label := map[domain.ItemKind]string{
				domain.ItemKindSeasoning: "调料",
			}[it.Kind]; label != "" {
				ui.Badge(c, label)
			}
		})

		if it.IntroWhat != "" {
			ui.Text(c, it.IntroWhat).FontSize(13).TextColor(p.Text).LineHeight(1.5)
		}
		rows := []struct{ label, text string }{
			{"怎么用", it.IntroHow}, {"怎么存", it.IntroStore}, {"小贴士", it.IntroTip},
		}
		for _, row := range rows {
			if row.text == "" {
				continue
			}
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, row.label).FontSize(11).FontWeight(700).TextColor(p.Muted)
				ui.Text(c, row.text).FontSize(12).TextColor(p.Text).LineHeight(1.45)
			})
		}
		if it.Note != "" {
			ui.Text(c, "备注："+it.Note).FontSize(12).TextColor(p.Muted)
		}

		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			if ui.Button(c, "更新数量").Clicked() {
				a.quickEdit(it)
			}
			if it.IsDepleted {
				if ui.Button(c, "恢复库存").Clicked() {
					a.setItemDepleted(it.ID, false)
					a.detailOpen = false
				}
			} else if ui.PrimaryButton(c, "标记用完").Clicked() {
				a.setItemDepleted(it.ID, true)
				a.detailOpen = false
			}
			if design.Button(c, p, "item-remove", "移出仓库",
				design.ButtonDanger).Clicked() {
				a.deleteItem(it.ID)
			}
			if ui.Button(c, "关闭").Clicked() {
				a.detailOpen = false
			}
		})
	})
}

func zoneName(it domain.Item) string {
	if domain.ZoneOf(it) == domain.ZoneStaple {
		return "底料架"
	}
	return "主菜区"
}

func (a *App) quickEditModal(c *ui.Context, p design.Palette) {
	if !a.editOpen {
		return
	}
	it, ok := a.itemsByID[a.editID]
	if !ok {
		a.editOpen = false
		return
	}
	design.ModalDialog(c, p, &a.editOpen, 460, a.level, func() {
		ui.Text(c, "更新 "+it.Name).FontSize(18).FontWeight(700).TextColor(p.Text)
		if design.Segmented(c, &a.editMode, "精确数量", "数量档位").Changed() {
			// The two editors share the modal; the choice applies at once.
		}
		if a.editMode == 0 {
			ui.Row(c).Gap(8).Children(func() {
				ui.DecimalInput(c, &a.editQtyText, "0", "9999", "0.1").
					Width(150).Label("数量")
				ui.TextInput(c, &a.editUnit).Placeholder("单位，如 个 / g").Width(180)
			})
		} else {
			if design.Segmented(c, &a.editGranular, "充足", "还有一些", "快没了").Changed() {
				// Read by saveQuickEdit.
			}
		}
		ui.Text(c, "保存后仓库会立即刷新。").FontSize(11).TextColor(p.Muted)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.editOpen = false
			}
			if ui.PrimaryButton(c, "保存").Clicked() {
				a.saveQuickEdit()
			}
		})
	})
}

func (a *App) customItemModal(c *ui.Context, p design.Palette) {
	if !a.customOpen {
		return
	}
	design.ModalDialog(c, p, &a.customOpen, 480, a.level, func() {
		ui.Text(c, "自建条目").FontSize(18).FontWeight(700).TextColor(p.Text)
		ui.Text(c, "只在本机保存，不会写进系统目录。").FontSize(11).TextColor(p.Muted)
		ui.Field(c, "名称", func() {
			ui.TextInput(c, &a.customName).Placeholder("例如 外婆的辣酱")
		})
		ui.Field(c, "分类", func() {
			ui.TextInput(c, &a.customCat).Placeholder("蔬菜 / 调料 / 未分类")
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				ui.Field(c, "数量", func() {
					ui.DecimalInput(c, &a.customQty, "0", "9999", "0.1").Label("数量")
				})
			})
			ui.Column(c).Grow(1).Children(func() {
				ui.Field(c, "单位", func() {
					ui.TextInput(c, &a.customUnit).Placeholder("个 / g / 瓶")
				})
			})
			ui.Column(c).Grow(1).Children(func() {
				ui.Field(c, "保质期（天）", func() {
					ui.TextInput(c, &a.customShelf).Placeholder("7")
				})
			})
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.customOpen = false
			}
			if ui.PrimaryButton(c, "加入仓库").Clicked() {
				a.addCustomItem()
			}
		})
	})
}

func (a *App) pantryCatalogPage(c *ui.Context, p design.Palette) {
	ui.Column(c).Grow(1).FillWidth().MinWidth(0).Children(func() {
		a.pageFrame(c, func() {
			ui.Column(c).FillWidth().Gap(10).Padding(0, 0, 10, 0).Children(func() {
				a.subHeader(c, p, "系统食材库", fmt.Sprintf("%d 条", len(a.catalogView)))
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					if ui.SearchField(c, &a.catalogQuery).Width(320).
						Placeholder("搜索 1,000 条系统目录").Label("搜索系统目录").Changed() {
						a.recomputeCatalogView()
					}
					ui.Text(c, "点到哪条加哪条；已存在会提示。").
						FontSize(design.FontMeta).TextColor(p.Faint)
					ui.Box(c).Grow(1)
					if design.Button(c, p, "catalog-find", "查找",
						design.ButtonGhost).Clicked() {
						a.catalogFindOpen = true
					}
				})
				matches := []int{}
				find := strings.ToLower(strings.TrimSpace(a.catalogFindQuery))
				if find != "" {
					for i, fi := range a.catalogView {
						if strings.Contains(strings.ToLower(fi.Name), find) {
							matches = append(matches, i)
						}
					}
				}
				if ui.FindBar(c, &a.catalogFindOpen, &a.catalogFindQuery,
					len(matches), &a.catalogFindIndex).Changed() && len(matches) > 0 {
					idx := a.catalogFindIndex
					if idx < 0 {
						idx = 0
					}
					if idx >= len(matches) {
						idx = len(matches) - 1
					}
					a.catalogList.ScrollTo(matches[idx]/2, ui.Start)
				}
			})
			ui.GridView(c.Key("catalog-grid"), &a.catalogGrid, len(a.catalogView),
				320, 96, func(i int) {
					row := a.catalogRow(c, p, a.catalogView[i])
					design.StaggerRow(row, design.Stagger(c, a.level, a.pageAt, i))
				}).Grow(1).FillWidth().MinWidth(0).
				Padding(design.PagePadding, 0, design.PagePadding, 0)
		})
	})
	a.itemDetailModal(c, p)
	a.quickEditModal(c, p)
}

func (a *App) catalogRow(c *ui.Context, p design.Palette, fi domain.FoodItem) ui.Element {
	owned := false
	for _, it := range a.items {
		if it.Name == fi.Name && !it.IsDepleted {
			owned = true
			break
		}
	}
	row := ui.Row(c).Grow(1).MinWidth(0).Height(88).Padding(design.Sp(c, 2.5)).Gap(design.Sp(c, 2.5)).
		Radius(design.RadiusSmall).Border(1, p.Border)
	design.Wash(row, p.Surface, p.Hover)
	design.Lift(row, 2, 0.98)
	row.Children(func() {
		key := ""
		if k, ok := assets.ItemImage(fi.Name); ok {
			key = k
		}
		if key != "" {
			a.images.Request(key, func() { a.post(func() {}) })
		}
		img := ui.Box(c).Width(52).Height(52).Radius(design.RadiusSmall).Clip().Center().
			Background(p.Hover)
		img.Children(func() {
			bm := a.images.Get(key)
			in := design.ImageIn(img, bm)
			if in < 1 {
				ui.Text(c, firstRune(fi.Name)).FontSize(20).FontWeight(700).
					TextColor(p.Accent.Alpha(0.3)).Opacity(1 - in)
			}
			design.DrawImage(c, bm, design.RadiusSmall, in)
		})
		ui.Column(c).Grow(1).MinWidth(0).Justify(ui.Center).Gap(2).Children(func() {
			ui.Text(c, fi.Name).FontSize(design.FontBody).FontWeight(600).
				TextColor(p.Text).SingleLine()
			meta := fi.Category
			if fi.ShelfLifeDays != nil {
				meta += fmt.Sprintf(" · %d 天", *fi.ShelfLifeDays)
			}
			ui.Text(c, meta).FontSize(design.FontMeta).TextColor(p.Muted).SingleLine()
		})
		if owned {
			ui.Text(c, "已在仓库").FontSize(design.FontMeta).TextColor(p.Faint)
		} else if design.Button(c, p, "add-"+fi.Name, "添加", design.ButtonSecondary).Clicked() {
			a.addCatalogItem(fi)
		}
	})
	return row
}

func (a *App) starterPackPage(c *ui.Context, p design.Palette) {
	a.pageFrame(c, func() {
		a.subHeader(c, p, "新手包", fmt.Sprintf("%d 个地域模板", len(a.packs)))
		ui.Text(c, "选一个常做的地域，把该地域家庭常备的底料一次放进仓库。可选条目默认不加。").
			FontSize(design.FontSmall).TextColor(p.Muted).Margin(0, 0, design.Sp(c, 3), 0)

		var selected domain.StarterPack
		for _, pack := range a.packs {
			if pack.ID == a.packID {
				selected = pack
				break
			}
		}

		ui.Split(c, &a.packSplit, func() {
			ui.List(c.Key("pack-list"), &a.packList, len(a.packs), func(i int) {
				pack := a.packs[i]
				row := ui.ButtonBase(c.Key("pack-"+pack.ID)).Row().FillWidth().
					Height(40).Padding(0, design.Sp(c, 2)).Gap(design.Sp(c, 1.5)).
					Radius(design.RadiusSmall).AlignItems(ui.Center).Label(pack.Region)
				rest := p.Surface.Alpha(0)
				if pack.ID == a.packID {
					rest = p.AccentPale
				}
				design.Wash(row, rest, p.Hover)
				row.Children(func() {
					color := p.Text
					if pack.ID == a.packID {
						color = p.Accent
					}
					ui.Text(c, pack.Region).FontSize(design.FontSmall).FontWeight(600).
						TextColor(color).Grow(1).SingleLine()
					ui.Text(c, fmt.Sprintf("%d+%d", len(pack.Core), len(pack.Optional))).
						FontSize(design.FontMeta).TextColor(p.Faint)
				})
				if row.Clicked() && pack.ID != a.packID {
					a.selectPack(pack)
				}
			})
		}, func() {
			if a.packID == "" {
				ui.Text(c, "先选一个模板，再确认要加哪些。").FontSize(design.FontSmall).
					TextColor(p.Faint).Padding(design.Sp(c, 4))
				return
			}
			ui.Scroll(c).Fill().Children(func() {
				a.packDetail(c, p, selected)
			})
		}).Grow(1)
	})
	a.quickEditModal(c, p)
}

// selectPack switches the chosen template and starts every optional item
// on: the master switch then decides whether any of them are added.
func (a *App) selectPack(pack domain.StarterPack) {
	a.packID = pack.ID
	a.packOptional = false
	a.packPicks = make(map[string]bool, len(pack.Optional))
	for _, name := range pack.Optional {
		a.packPicks[name] = true
	}
}

// packDetail is the right pane: what the template holds, which optional
// items to take, and the one button the page exists for.
func (a *App) packDetail(c *ui.Context, p design.Palette, pack domain.StarterPack) {
	ui.Column(c).FillWidth().Padding(design.Sp(c, 3), 0).Gap(design.Sp(c, 2.5)).Children(func() {
		ui.Text(c, pack.Region).FontSize(design.FontHeading).FontWeight(700).TextColor(p.Text).Role(ui.RoleHeading)
		ui.Text(c, fmt.Sprintf("常备 %d · 可选 %d", len(pack.Core), len(pack.Optional))).
			FontSize(design.FontMeta).TextColor(p.Muted)

		ui.Text(c, "常备底料").FontSize(design.FontMeta).FontWeight(700).TextColor(p.Muted)
		ui.Row(c).FillWidth().Gap(design.Sp(c, 1)).Wrap().Children(func() {
			for _, name := range pack.Core {
				ui.Badge(c, name).FontSize(design.FontMeta).Padding(3, 8).
					Radius(design.RadiusSmall).Background(p.SurfaceAlt).TextColor(p.Text)
			}
		})

		ui.Divider(c)

		ui.Form(c, func() {
			ui.Field(c, "可选条目", func() {
				if len(pack.Optional) == 0 {
					ui.Text(c, "这个模板没有可选条目。").FontSize(design.FontSmall).
						TextColor(p.Faint)
					return
				}
				ui.Row(c).FillWidth().Gap(design.Sp(c, 1)).Wrap().Children(func() {
					for _, name := range pack.Optional {
						name := name
						on := a.packPicks[name]
						if ui.Toggle(c, &on, name).Changed() {
							a.packPicks[name] = on
						}
					}
				})
			}).Description("打开下面的开关才会一起入库；单个条目可以逐个关掉。")
			ui.Field(c, "同时加入可选条目", func() {
				ui.Switch(c, &a.packOptional)
			}).Description("默认只加常备底料；打开后连可选条目一起入库。")
		})

		ui.Row(c).Gap(design.Sp(c, 2)).Margin(design.Sp(c, 1), 0, 0, 0).Children(func() {
			if ui.PrimaryButton(c, "加入这些食材").Clicked() {
				a.addPack(pack)
			}
			if ui.Button(c, "回到仓库").Clicked() {
				a.router.Replace("/pantry")
			}
		})
	})
}

func (a *App) shoppingPage(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		a.subHeader(c, p, "购物清单", fmt.Sprintf("%d 项待买", len(a.shopping)))
		ui.Text(c, "已用完的条目，加上已加入掌序、但仓库里没有的必需食材。换算成购物数量后可直接加回仓库。").FontSize(12).TextColor(p.Muted)

		if len(a.shopping) == 0 {
			ui.Column(c).FillWidth().Padding(30).Gap(6).Radius(16).
				Background(p.Surface).Border(1, p.Border).Children(func() {
				ui.Text(c, "清单是空的").FontSize(18).FontWeight(700).TextColor(p.Text)
				ui.Text(c, "把菜加入掌序后，缺的食材会自动出现在这里。").FontSize(12).TextColor(p.Muted)
			})
			return
		}
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			for _, line := range a.shopping {
				line := line
				ui.Row(c).FillWidth().Height(56).Padding(12, 8).Gap(10).AlignItems(ui.Center).
					Radius(12).Background(p.Surface).Border(1, p.Border).
					Role(ui.RoleListItem).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, line.Name).FontSize(13).FontWeight(600).TextColor(p.Text)
						detail := ""
						if line.Depleted {
							detail = "仓库里已用完"
						} else if line.Recipe != "" {
							detail = fmt.Sprintf("做「%s」需要 %s%s", line.Recipe, trimFloat(line.Needed), line.Unit)
						}
						if detail != "" {
							ui.Text(c, detail).FontSize(11).TextColor(p.Muted)
						}
					})
					if line.Depleted {
						if ui.Button(c, "恢复").Clicked() {
							for _, it := range a.items {
								if it.Name == line.Name && it.IsDepleted {
									a.setItemDepleted(it.ID, false)
									break
								}
							}
						}
					} else if fi, ok := a.catalogIndex[line.Name]; ok {
						if ui.Button(c, "加回仓库").Clicked() {
							a.addCatalogItem(fi)
						}
					}
				})
			}
		})
	})
	a.itemDetailModal(c, p)
	a.quickEditModal(c, p)
}

func (a *App) reconcilePage(c *ui.Context, p design.Palette) {
	ui.Column(c).Grow(1).FillWidth().MinWidth(0).Children(func() {
		a.pageFrame(c, func() {
			ui.Column(c).FillWidth().Gap(6).Padding(0, 0, 10, 0).Children(func() {
				a.subHeader(c, p, "盘点核销", fmt.Sprintf("%d 条在库", len(a.items)))
				ui.Text(c, "逐项确认：还在就点「还有」，用完了点「用完」。用完的条目保留为灰卡，可随时恢复。").
					FontSize(design.FontSmall).TextColor(p.Muted)
			})
			rows := len(a.items)
			a.listBody(c, "reconcile-list", &a.reconcileList, rows, func(i int) {
				it := a.items[i]
				row := ui.Row(c).FillWidth().Height(58).Padding(12, 8).Gap(10).
					AlignItems(ui.Center).Radius(design.RadiusSmall+4).Border(1, p.Border).
					Role(ui.RoleListItem)
				design.Wash(row, p.Surface, p.Hover)
				design.RestShadow(row, p)
				row.Children(func() {
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, it.Name).FontSize(13).FontWeight(600).TextColor(p.Text)
						ui.Text(c, zoneName(it)+" · "+it.RemainingText()).
							FontSize(design.FontMeta).TextColor(p.Muted)
					})
					if it.IsDepleted {
						ui.Text(c, "已用完").FontSize(design.FontMeta).TextColor(p.Faint)
						if design.Button(c, p, fmt.Sprintf("restore-%d", it.ID), "恢复",
							design.ButtonSecondary).Clicked() {
							a.setItemDepleted(it.ID, false)
						}
					} else {
						if design.Button(c, p, fmt.Sprintf("keep-%d", it.ID), "还有",
							design.ButtonGhost).Clicked() {
							a.notice = it.Name + " 仍在库"
						}
						if design.Button(c, p, fmt.Sprintf("done-%d", it.ID), "用完",
							design.ButtonDanger).Clicked() {
							a.setItemDepleted(it.ID, true)
							a.notice = it.Name + " 已核销"
						}
					}
				})
			})
		})
	})
}

// subHeader is a secondary page's own heading: the leaf title and one
// count or action. The way back lives in the title bar, so this row does
// not draw a second back button.
func (a *App) subHeader(c *ui.Context, p design.Palette, title, sub string) {
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Margin(0, 0, 12, 0).Children(func() {
		ui.Text(c, title).FontSize(design.FontHeading).FontWeight(700).TextColor(p.Text).
			Role(ui.RoleHeading)
		ui.Text(c, sub).FontSize(design.FontSmall).TextColor(p.Muted)
	})
}

// storageLabel names a storage location for the compact card's meta line.
func storageLabel(l domain.StorageLocation) string {
	switch l {
	case domain.StorageFridge:
		return "冷藏"
	case domain.StorageFreezer:
		return "冷冻"
	case domain.StorageRoomTemp:
		return "常温"
	}
	return ""
}
