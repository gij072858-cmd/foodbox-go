package app

// M3 · 菜式编辑器与版本组。
//
// This file is the M3 workstream's home: the editor state, the create/edit
// page, the save/delete transactions and the version-group helpers. The
// list and detail pages live in recipes_views.go.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
	"github.com/gij072858-cmd/foodbox-go/internal/services/season"
)

// The recipe list's sort modes, in the order the segmented control shows
// them. They are applied when the view is recomputed, never while drawing.
const (
	recipeSortDefault = iota
	recipeSortFastest
	recipeSortNewest
)

// The recipe taxonomy, the same three dimensions the seed uses: what kind
// of dish it is, where it comes from, and what the main ingredient is.
var (
	recipeCuisineOptions = []string{"家常菜", "快手菜", "汤羹", "凉菜"}
	recipeRegionOptions  = []string{
		"川菜", "粤菜", "湘菜", "鲁菜", "苏菜", "浙菜", "闽菜", "徽菜",
		"东北菜", "山西菜", "潮汕菜", "客家菜", "云贵菜", "西北菜",
		"京津菜", "西餐", "东南亚菜", "日韩菜",
	}
	recipeMainCategoryOptions = []string{
		"猪肉", "牛肉", "羊肉", "鸡肉", "鸭肉", "鱼类", "虾蟹贝",
		"其他海鲜", "蔬菜", "菌菇", "蛋类", "豆制品", "主食", "面点",
	}
)

// stepResourceOptions are the step resources the editor can tag and the
// scheduler reads. The order is the order the chips show.
var stepResourceOptions = []struct{ Name, Label string }{
	{"stove", "灶台"},
	{"board", "案板"},
	{"hand", "手工"},
	{"waiting", "等待"},
}

// editIngredient is one draft row of the ingredient table. A row with an
// empty name is dropped on save, so an unused blank row is harmless.
type editIngredient struct {
	name      string
	amount    float64
	unit      string
	grams     float64 // 0 and below means "not measured in grams"
	required  bool
	seasoning bool
}

// editStep is one draft step. The fields the editor does not expose
// (safety tag, notes, photos, video) are carried through unchanged so
// editing a seeded dish does not silently erase them.
type editStep struct {
	instruction string
	minutes     float64
	stove       bool
	board       bool
	hand        bool
	waiting     bool
	cue         string

	safety string
	note   string
	photos []string
	video  string
}

// recipeEditState is the M3 feature block: the editor's form, the list's
// seasonal filter and sort, and the precomputed version-group index. All
// of it is rebuilt after a load or a write, never while drawing.
type recipeEditState struct {
	// Editor.
	editing   int64 // 0 creates a new dish
	loaded    bool  // the form has been filled for loadedFor
	loadedFor int64
	saving    bool
	err       string

	// splitWidth is the form pane's width on the editor page.
	splitWidth float32

	name         string
	cuisine      string
	region       string
	mainCategory string
	duration     float64
	difficulty   int
	servings     float64
	months       [12]bool
	intro        string
	platform     string
	author       string
	sourceURL    string
	versionGroup string
	isDish       bool
	ingredients  []editIngredient
	steps        []editStep

	// Deletions are requested while a row builds and applied after the
	// table, so a row's slice is never mutated mid-iteration.
	removeIngredient int
	removeStep       int

	pendingDelete int64
	confirmDelete bool

	// List view. The data changes rarely, so this is computed once per
	// change instead of per row. The four filters are values from the
	// taxonomy; an empty one means "not limited".
	seasonOnly    bool
	sortMode      int
	seasonMonth   int
	mainFilter    string
	regionFilter  string
	typeFilter    string
	sortFilter    int
	lastTableSort ui.SortOrder
	// savedColumns is the table layout last written to prefs, so a drag
	// saves once instead of every frame.
	savedColumns ui.TableLayout
	versionPick  string
	versions     map[string][]int64
	versionOf    map[int64]string
}

func (s *recipeEditState) init() {
	s.versions = map[string][]int64{}
	s.versionOf = map[int64]string{}
	s.seasonMonth = season.CurrentMonth(time.Now())
	s.splitWidth = 640
	s.reset()
}

// loadRecipeState is a no-op today: dishes come with the base snapshot and
// the version index is rebuilt from it. It exists so a restore can refresh
// editor-only state later.
func (a *App) loadRecipeState(_ context.Context, _ repository.Repository) error { return nil }

// reset clears the editor back to a fresh form. It leaves the list filters
// and the version index alone.
func (s *recipeEditState) reset() {
	s.editing = 0
	s.loaded = false
	s.loadedFor = -1
	s.saving = false
	s.err = ""
	s.name = ""
	s.cuisine = ""
	s.region = ""
	s.mainCategory = ""
	s.duration = 20
	s.difficulty = 0
	s.servings = 2
	s.months = [12]bool{}
	s.intro = ""
	s.platform = ""
	s.author = ""
	s.sourceURL = ""
	s.versionGroup = ""
	s.isDish = true
	s.ingredients = []editIngredient{{required: true}}
	s.steps = []editStep{{hand: true}}
	s.removeIngredient = -1
	s.removeStep = -1
	s.pendingDelete = 0
	s.confirmDelete = false
}

// load fills the form for id; 0 starts a new dish. It only runs when the
// page opens or the route changes, not on every frame.
func (s *recipeEditState) load(id int64, byID map[int64]domain.Recipe, steps map[int64][]domain.Step) {
	s.reset()
	s.editing = id
	s.loaded = true
	s.loadedFor = id
	if id == 0 {
		return
	}
	rec, ok := byID[id]
	if !ok {
		s.err = "这道菜不存在，可能已被删除。"
		return
	}
	s.name = rec.Name
	s.cuisine = rec.Cuisine
	s.region = rec.Region
	s.mainCategory = rec.MainCategory
	if rec.DurationMinutes != nil {
		s.duration = *rec.DurationMinutes
	}
	switch rec.Difficulty {
	case domain.DifficultyMedium:
		s.difficulty = 1
	case domain.DifficultyHard:
		s.difficulty = 2
	}
	if rec.BaseServings > 0 {
		s.servings = float64(rec.BaseServings)
	}
	for _, m := range rec.SeasonMonths {
		if m >= 1 && m <= 12 {
			s.months[m-1] = true
		}
	}
	s.intro = rec.Intro
	if rec.Source != nil {
		s.platform = rec.Source.Platform
		s.author = rec.Source.Author
		s.sourceURL = rec.Source.URL
	}
	s.versionGroup = rec.VersionGroup
	s.isDish = rec.IsDish
	s.ingredients = make([]editIngredient, 0, len(rec.Ingredients))
	for _, in := range rec.Ingredients {
		row := editIngredient{
			name: in.Name, amount: in.Amount, unit: in.Unit,
			required: in.Required, seasoning: in.Seasoning,
		}
		if in.Grams != nil {
			row.grams = *in.Grams
		}
		s.ingredients = append(s.ingredients, row)
	}
	if len(s.ingredients) == 0 {
		s.ingredients = []editIngredient{{required: true}}
	}
	s.steps = make([]editStep, 0, len(steps[id]))
	for _, st := range steps[id] {
		row := editStep{
			instruction: st.Instruction,
			cue:         st.CompletionCue,
			safety:      st.SafetyTag,
			note:        st.Note,
			photos:      st.PhotoPaths,
			video:       st.VideoPath,
			hand:        true,
		}
		if st.DurationMinutes != nil {
			row.minutes = *st.DurationMinutes
		}
		for _, res := range st.Resources {
			switch res {
			case "stove":
				row.stove = true
				row.hand = false
			case "board":
				row.board = true
				row.hand = false
			case "hand":
				row.hand = true
			case "waiting":
				row.waiting = true
			}
		}
		s.steps = append(s.steps, row)
	}
	if len(s.steps) == 0 {
		s.steps = []editStep{{hand: true}}
	}
}

// selectedMonths returns the form's months as the seed stores them: nothing
// selected means all-year supply.
func (s *recipeEditState) selectedMonths() []int {
	out := make([]int, 0, 12)
	for m := 1; m <= 12; m++ {
		if s.months[m-1] {
			out = append(out, m)
		}
	}
	return out
}

// formRecipe turns the form into a dish and its steps. The error is the
// message the form shows; when it returns one nothing is written.
func (s *recipeEditState) formRecipe() (domain.Recipe, []domain.Step, error) {
	name := strings.TrimSpace(s.name)
	if name == "" {
		return domain.Recipe{}, nil, fmt.Errorf("请填写菜式名称")
	}
	ingredients := make([]domain.Ingredient, 0, len(s.ingredients))
	for _, in := range s.ingredients {
		inName := strings.TrimSpace(in.name)
		if inName == "" {
			continue
		}
		if in.amount < 0 || in.grams < 0 {
			return domain.Recipe{}, nil, fmt.Errorf("「%s」的数量不能为负", inName)
		}
		ing := domain.Ingredient{
			Name: inName, Amount: in.amount, Unit: strings.TrimSpace(in.unit),
			Required: in.required, Seasoning: in.seasoning,
		}
		if in.grams > 0 {
			grams := in.grams
			ing.Grams = &grams
		}
		ingredients = append(ingredients, ing)
	}
	steps := make([]domain.Step, 0, len(s.steps))
	for _, st := range s.steps {
		text := strings.TrimSpace(st.instruction)
		if text == "" {
			continue
		}
		if st.minutes < 0 {
			return domain.Recipe{}, nil, fmt.Errorf("步骤时长不能为负")
		}
		step := domain.Step{
			OrderIndex:    len(steps) + 1,
			Instruction:   text,
			Resources:     st.resourceList(),
			SafetyTag:     st.safety,
			PhotoPaths:    st.photos,
			Note:          st.note,
			VideoPath:     st.video,
			CompletionCue: strings.TrimSpace(st.cue),
		}
		if st.minutes > 0 {
			minutes := st.minutes
			step.DurationMinutes = &minutes
		}
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return domain.Recipe{}, nil, fmt.Errorf("至少写一步步骤")
	}
	if s.servings < 1 {
		return domain.Recipe{}, nil, fmt.Errorf("份数至少为 1")
	}
	if s.duration < 0 {
		return domain.Recipe{}, nil, fmt.Errorf("时长不能为负")
	}
	rec := domain.Recipe{
		ID:           s.editing,
		Name:         name,
		Cuisine:      strings.TrimSpace(s.cuisine),
		Region:       strings.TrimSpace(s.region),
		MainCategory: strings.TrimSpace(s.mainCategory),
		Difficulty:   []domain.Difficulty{domain.DifficultyEasy, domain.DifficultyMedium, domain.DifficultyHard}[s.difficulty],
		BaseServings: int(s.servings),
		SeasonMonths: s.selectedMonths(),
		Ingredients:  ingredients,
		Intro:        strings.TrimSpace(s.intro),
		VersionGroup: strings.TrimSpace(s.versionGroup),
		IsDish:       s.isDish,
		Steps:        steps,
	}
	if s.duration > 0 {
		minutes := s.duration
		rec.DurationMinutes = &minutes
	}
	platform := strings.TrimSpace(s.platform)
	author := strings.TrimSpace(s.author)
	url := strings.TrimSpace(s.sourceURL)
	if platform != "" || author != "" || url != "" {
		rec.Source = &domain.RecipeSource{Platform: platform, Author: author, URL: url}
	}
	return rec, steps, nil
}

// resourceList returns the step's resources in the scheduler's vocabulary
// and the order the chips show.
func (st editStep) resourceList() []string {
	on := map[string]bool{
		"stove": st.stove, "board": st.board, "hand": st.hand, "waiting": st.waiting,
	}
	out := make([]string, 0, 4)
	for _, opt := range stepResourceOptions {
		if on[opt.Name] {
			out = append(out, opt.Name)
		}
	}
	return out
}

// openRecipeEditor resets the form and opens it; id 0 creates a dish.
func (a *App) openRecipeEditor(id int64) {
	a.recipeEdit.reset()
	if id == 0 {
		a.router.Push("/recipes/new")
		return
	}
	a.router.Push(fmt.Sprintf("/recipes/%d/edit", id))
}

// recipeEditorPage is the create/edit form. id == 0 creates a new dish.
func (a *App) recipeEditorPage(c *ui.Context, p design.Palette, id int64) {
	s := &a.recipeEdit
	if !s.loaded || s.loadedFor != id {
		s.load(id, a.recipeByID, a.steps)
	}
	ui.Column(c).Grow(1).FillWidth().MinWidth(0).Children(func() {
		ui.Split(c, &s.splitWidth, func() {
			a.pageBody(c, func() {
				a.editorHeader(c, p)
				if s.err != "" {
					ui.Text(c, s.err).FontSize(12).FontWeight(600).TextColor(p.Danger).
						Margin(0, 0, 12, 0)
				}
				a.editorBasics(c, p)
				a.editorSeason(c, p)
				a.editorIntro(c, p)
				a.editorIngredients(c, p)
				a.editorSteps(c, p)
				// Room for the fixed save bar to sit over nothing.
				ui.Box(c).Height(72)
			})
		}, func() {
			a.editorPreview(c, p)
		}).Grow(1).FillWidth()
		a.editorSaveBar(c, p)
		a.recipeDeleteDialog(c, p)
	})
}

// editorPreview is the live card the form is building: the generated
// cover, the name and the counts, updating as the fields change.
func (a *App) editorPreview(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	name := strings.TrimSpace(s.name)
	if name == "" {
		name = "未命名菜式"
	}
	rec := domain.Recipe{
		Name: name, Cuisine: s.cuisine, Region: s.region,
		MainCategory: s.mainCategory,
	}
	ui.Scroll(c).Fill().Padding(20, 20, 88, 20).Children(func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			ui.Text(c, "预览").FontSize(design.FontHeading).FontWeight(700).TextColor(p.Text).Role(ui.RoleHeading)
			card := design.Card(c, p).Padding(14).Gap(10)
			card.Children(func() {
				// The preview is the list row itself: the same mark, the same
				// type, so what you write is what the 菜式 page will show.
				row := ui.Row(c).FillWidth().Height(76).Padding(design.Sp(c, 2), design.Sp(c, 2.5), design.Sp(c, 2), design.Sp(c, 3)).Gap(design.Sp(c, 2.5)).
					Radius(design.RadiusCard).Border(1, p.Border).Background(p.Surface).Clip()
				row.Children(func() {
					a.recipeMarkTile(c, p, rec, 34)
					ui.Column(c).Grow(1).MinWidth(0).Justify(ui.Center).Gap(design.Sp(c, 0.75)).Children(func() {
						ui.Text(c, name).FontSize(design.FontSubhead).FontWeight(600).
							TextColor(p.Text).SingleLine()
						meta := recipeMeta(rec)
						if meta != "" {
							ui.Text(c, meta).FontSize(design.FontMeta).TextColor(p.Muted).SingleLine()
						}
						ui.Text(c, fmt.Sprintf("%d 样食材 · %d 步", len(s.ingredients), len(s.steps))).
							FontSize(design.FontMeta).TextColor(p.Faint)
					})
				})
			})
			ui.Text(c, "拖动中间的分隔线可以调整预览宽度。").
				FontSize(design.FontMeta).TextColor(p.Faint)
		})
	})
}

// editorSection is one titled card of the form, so a long dish reads as
// four groups rather than a wall of fields.
func (a *App) editorSection(c *ui.Context, p design.Palette, title string, fn func()) {
	card := design.CardPad(c, p).Margin(0, 0, 18, 0)
	card.Children(func() {
		ui.Fieldset(c, title, fn)
	})
}

// editorSaveBar keeps the primary action in reach at the bottom of a long
// form, with the top bar left to navigation and delete.
func (a *App) editorSaveBar(c *ui.Context, p design.Palette) {
	bar := ui.Row(c.Key("editor-save-bar")).Absolute().Bottom(0).Left(0).Right(0).Height(64).
		Material(design.BarMaterial(p)).Shadow(0, -8, 22, -14, p.Shadow)
	bar.Children(func() {
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(1).Background(p.Border)
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Row(c).FillWidth().MaxWidth(design.PageMaxWidth).Padding(0, design.PagePadding).
				AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				ui.Text(c, "名称必填，至少写一步步骤；保存时菜式与步骤一起写入。").
					FontSize(design.FontMeta).TextColor(p.Muted).SingleLine()
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if design.Button(c, p, "editor-cancel", "取消",
						design.ButtonGhost).Clicked() {
						a.router.Back()
					}
					if design.Button(c, p, "editor-save", "保存菜式",
						design.ButtonPrimary).Clicked() {
						a.saveRecipe()
					}
				})
			})
		})
	})
}

// editorHeader draws the breadcrumb, the primary save action and, for an
// existing dish, its delete entry.
func (a *App) editorHeader(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	title := "新建菜式"
	if s.editing != 0 {
		title = "编辑菜式"
	}
	ui.Row(c).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).
		Margin(0, 0, 12, 0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.BackButton(c, a.router)
			chosen := 1
			if ui.Breadcrumbs(c, []string{"菜式", title}, &chosen).Changed() && chosen == 0 {
				a.router.Replace("/recipes")
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			if s.editing != 0 {
				if design.Button(c, p, "editor-delete", "删除菜式",
					design.ButtonDanger).Clicked() {
					a.askDeleteRecipe(s.editing)
				}
			}
		})
	})
	ui.Text(c, title).FontSize(26).FontWeight(700).TextColor(p.Text).Margin(0, 0, 6, 0)
	ui.Text(c, "名称必填，至少写一步步骤；保存时菜式与步骤一起写入。").FontSize(12).
		TextColor(p.Muted).Margin(0, 0, 18, 0)
}

func (a *App) editorBasics(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	a.editorSection(c, p, "基本信息", func() {
		ui.Row(c).FillWidth().Gap(14).Children(func() {
			ui.Field(c, "菜式名称", func() {
				ui.TextInput(c, &s.name).Placeholder("比如 番茄炖牛腩")
			}).Grow(2)
			ui.Field(c, "版本组", func() {
				ui.TextInput(c, &s.versionGroup).Placeholder("同名多做法时填同一个组名")
			}).Grow(1)
		})
		ui.Row(c).FillWidth().Gap(14).Margin(12, 0, 0, 0).Children(func() {
			a.editorSelect(c, p, "菜系", recipeCuisineOptions, &s.cuisine)
			a.editorSelect(c, p, "地域", recipeRegionOptions, &s.region)
			a.editorSelect(c, p, "主料分类", recipeMainCategoryOptions, &s.mainCategory)
		})
		ui.Row(c).FillWidth().Gap(14).AlignItems(ui.End).Margin(12, 0, 0, 0).Children(func() {
			ui.Field(c, "时长（分钟）", func() {
				ui.NumberInput(c, &s.duration, 0, 600, 5)
			}).Width(150)
			ui.Field(c, "份数", func() {
				ui.NumberInput(c, &s.servings, 1, 20, 1)
			}).Width(110)
			ui.Field(c, "难度", func() {
				design.Segmented(c, &s.difficulty, "简单", "中等", "困难")
			}).Width(240)
		})
		ui.Row(c).FillWidth().Margin(12, 0, 0, 0).Children(func() {
			ui.Field(c, "菜式", func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Switch(c, &s.isDish).Label("这是一道菜式")
					ui.Text(c, "这是一道菜式").FontSize(12).TextColor(p.Muted)
				})
			})
		})
	})
}

// editorSelect is a taxonomy drop-down with an explicit 「未指定」 choice,
// so clearing a dimension is possible rather than implied.
func (a *App) editorSelect(c *ui.Context, p design.Palette, label string, options []string, value *string) {
	choice := *value
	if choice == "" {
		choice = "未指定"
	}
	ui.Field(c, label, func() {
		if ui.Combobox(c, &choice, append([]string{"未指定"}, options...)).Changed() {
			if choice == "未指定" {
				*value = ""
			} else {
				*value = choice
			}
		}
	}).Grow(1)
}

func (a *App) editorSeason(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	a.editorSection(c, p, "时令", func() {
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			for m := 1; m <= 12; m++ {
				m := m
				chip := design.Chip(c, p, fmt.Sprintf("month-%d", m), fmt.Sprintf("%d 月", m), s.months[m-1])
				if chip.Clicked() {
					s.months[m-1] = !s.months[m-1]
				}
			}
		})
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
			ui.Text(c, "当前："+season.Label(s.selectedMonths())).FontSize(11).TextColor(p.Muted)
			ui.Text(c, "不选任何月份就是全年供应。").FontSize(11).TextColor(p.Faint)
			if ui.Button(c, "全不选").Clicked() {
				s.months = [12]bool{}
			}
		})
	})
}

func (a *App) editorIntro(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	a.editorSection(c, p, "简介与来源", func() {
		ui.Field(c, "简介", func() {
			ui.TextArea(c, &s.intro).Lines(2, 4).Placeholder("一两句话说清这道菜")
		})
		ui.Row(c).FillWidth().Gap(14).Margin(12, 0, 0, 0).Children(func() {
			ui.Field(c, "来源平台", func() { ui.TextInput(c, &s.platform).Placeholder("小红书 / 下厨房") }).Grow(1)
			ui.Field(c, "作者", func() { ui.TextInput(c, &s.author).Placeholder("原作者") }).Grow(1)
			ui.Field(c, "链接", func() { ui.TextInput(c, &s.sourceURL).Placeholder("https://") }).Grow(2)
		})
	})
}

func (a *App) editorIngredients(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	a.editorSection(c, p, fmt.Sprintf("食材（%d 行）", len(s.ingredients)), func() {
		for i := range s.ingredients {
			i := i
			in := &s.ingredients[i]
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
				ui.Field(c, fmt.Sprintf("食材 %d", i+1), func() {
					// The system catalog is the suggestion list: typing
					// 「番」 offers 番茄 and the other matches.
					ui.Autocomplete(c, &in.name, a.catalogNames).Placeholder("名称")
				}).Grow(2)
				ui.Field(c, "数量", func() {
					ui.NumberInput(c, &in.amount, 0, 9999, 1)
				}).Width(110)
				ui.Field(c, "单位", func() {
					ui.TextInput(c, &in.unit).Placeholder("g / 个")
				}).Width(90)
				ui.Field(c, "克数", func() {
					ui.NumberInput(c, &in.grams, 0, 5000, 10)
				}).Width(110)
				ui.Checkbox(c, &in.required, "必需")
				ui.Checkbox(c, &in.seasoning, "调料")
				a.deleteIngredientButton(c, p, i)
			})
		}
		if s.removeIngredient >= 0 && s.removeIngredient < len(s.ingredients) {
			at := s.removeIngredient
			s.ingredients = append(s.ingredients[:at], s.ingredients[at+1:]...)
			s.removeIngredient = -1
		}
		ui.Row(c).Gap(10).Margin(4, 0, 0, 0).Children(func() {
			if ui.Button(c, "添加食材").Clicked() {
				s.ingredients = append(s.ingredients, editIngredient{required: true})
			}
			ui.Text(c, "空着名称的行不会保存；克数用于营养计算，可留空。").FontSize(11).TextColor(p.Faint)
		})
	})
}

func (a *App) deleteIngredientButton(c *ui.Context, p design.Palette, i int) {
	btn := design.Button(c, p, fmt.Sprintf("ing-delete-%d", i), "删除",
		design.ButtonGhost).Height(30).Padding(4, 10)
	btn.Label(fmt.Sprintf("删除食材 %d", i+1))
	if btn.Clicked() {
		// The caller applies the removal after the table, not mid-loop.
		a.recipeEdit.removeIngredient = i
	}
}

func (a *App) editorSteps(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	a.editorSection(c, p, fmt.Sprintf("步骤（%d 步）", len(s.steps)), func() {
		for i := range s.steps {
			i := i
			st := &s.steps[i]
			ui.Column(c).FillWidth().Gap(8).Padding(12).Radius(12).
				Background(p.SurfaceAlt).Border(1, p.Border).Children(func() {
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.End).Children(func() {
					ui.Text(c, fmt.Sprintf("第 %d 步", i+1)).FontSize(12).FontWeight(700).
						TextColor(p.Accent).Width(52).Shrink(0)
					ui.Field(c, fmt.Sprintf("第 %d 步说明", i+1), func() {
						ui.TextInput(c, &st.instruction).Placeholder("这一步做什么")
					}).Grow(1)
					ui.Field(c, "时长（分钟）", func() {
						ui.NumberInput(c, &st.minutes, 0, 300, 1)
					}).Width(120)
					ui.Field(c, "完成标志", func() {
						ui.TextInput(c, &st.cue).Placeholder("颜色、气味、手感")
					}).Width(200)
				})
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "资源").FontSize(11).TextColor(p.Muted)
					a.stepResourceChips(c, p, i, st)
					ui.Box(c).Grow(1)
					a.deleteStepButton(c, p, i)
				})
			})
		}
		if s.removeStep >= 0 && s.removeStep < len(s.steps) {
			at := s.removeStep
			s.steps = append(s.steps[:at], s.steps[at+1:]...)
			s.removeStep = -1
		}
		ui.Row(c).Gap(10).Margin(4, 0, 0, 0).Children(func() {
			if ui.Button(c, "添加一步").Clicked() {
				s.steps = append(s.steps, editStep{hand: true})
			}
			ui.Text(c, "说明留空的步骤不会保存。").FontSize(11).TextColor(p.Faint)
		})
	})
}

func (a *App) stepResourceChips(c *ui.Context, p design.Palette, i int, st *editStep) {
	on := []*bool{&st.stove, &st.board, &st.hand, &st.waiting}
	for j, opt := range stepResourceOptions {
		opt := opt
		on := on[j]
		chip := design.Chip(c, p, fmt.Sprintf("step-%d-res-%s", i, opt.Name), opt.Label, *on)
		if chip.Clicked() {
			*on = !*on
		}
	}
}

func (a *App) deleteStepButton(c *ui.Context, p design.Palette, i int) {
	btn := design.Button(c, p, fmt.Sprintf("step-delete-%d", i), "删除这一步",
		design.ButtonGhost).Height(30).Padding(4, 10)
	btn.Label(fmt.Sprintf("删除第 %d 步", i+1))
	if btn.Clicked() {
		a.recipeEdit.removeStep = i
	}
}

// saveRecipe validates the form and writes it in one transaction, then
// reloads the snapshot. The write runs off the main thread when a window
// is up; a headless test saves inline so the tester sees the result at once.
func (a *App) saveRecipe() {
	s := &a.recipeEdit
	if s.saving {
		return
	}
	rec, steps, err := s.formRecipe()
	if err != nil {
		s.err = err.Error()
		return
	}
	store := a.store
	if store == nil {
		s.err = "数据库尚未就绪"
		return
	}
	s.saving = true
	s.err = ""
	rec.Steps = steps
	if a.win == nil {
		err := a.writeRecipe(store, rec)
		a.finishRecipeSave(rec.Name, err)
		return
	}
	go func() {
		err := a.writeRecipe(store, rec)
		a.post(func() { a.finishRecipeSave(rec.Name, err) })
	}()
}

// writeRecipe runs the single create-or-update transaction. Both arms write
// the steps with the recipe row.
func (a *App) writeRecipe(store repository.Repository, rec domain.Recipe) error {
	ctx := context.Background()
	if rec.ID == 0 {
		_, err := store.CreateRecipe(ctx, rec)
		return err
	}
	return store.UpdateRecipe(ctx, rec)
}

// finishRecipeSave runs on the main thread after the write. It refreshes
// the dish snapshot and returns to the list, where the saved dish shows.
func (a *App) finishRecipeSave(name string, err error) {
	s := &a.recipeEdit
	s.saving = false
	if err != nil {
		s.err = "保存失败：" + err.Error()
		return
	}
	a.notice = "已保存「" + name + "」"
	s.reset()
	a.reloadRecipes(func() {
		a.router.Replace("/recipes")
	})
}

// askDeleteRecipe opens the confirmation for one dish.
func (a *App) askDeleteRecipe(id int64) {
	s := &a.recipeEdit
	s.pendingDelete = id
	s.confirmDelete = true
}

// recipeDeleteDialog draws the confirmation while it is open; a confirmed
// delete runs the transaction and returns to the list.
func (a *App) recipeDeleteDialog(c *ui.Context, p design.Palette) {
	s := &a.recipeEdit
	if !s.confirmDelete {
		return
	}
	name := "这道菜"
	if rec, ok := a.recipeByID[s.pendingDelete]; ok {
		name = rec.Name
	}
	switch ui.AlertDialog(c, &s.confirmDelete, "删除「"+name+"」？",
		"删除后不能恢复；做过的记录会保留。", "取消", "删除") {
	case 1:
		a.deleteRecipeNow(s.pendingDelete)
	}
}

// deleteRecipeNow removes one dish. Like save, it runs inline in a headless
// test and on a goroutine with a window.
func (a *App) deleteRecipeNow(id int64) {
	store := a.store
	if id == 0 {
		return
	}
	if store == nil {
		a.notice = "数据库尚未就绪"
		return
	}
	if a.win == nil {
		err := store.DeleteRecipe(context.Background(), id)
		a.finishRecipeDelete(err)
		return
	}
	go func() {
		err := store.DeleteRecipe(context.Background(), id)
		a.post(func() { a.finishRecipeDelete(err) })
	}()
}

func (a *App) finishRecipeDelete(err error) {
	s := &a.recipeEdit
	s.pendingDelete = 0
	if err != nil {
		a.notice = "删除失败：" + err.Error()
		return
	}
	a.notice = "已删除"
	s.reset()
	a.reloadRecipes(func() {
		a.router.Replace("/recipes")
	})
}
