package sichu

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/nutrition"
)

// Style is the meal structure the user asked for.
type Style string

const (
	// StyleCombo is 两菜一汤: one protein dish, one vegetable dish, one soup.
	StyleCombo Style = "combo"
	// StyleOneBowl is 一碗端: one self-sufficient bowl, optionally one soup.
	StyleOneBowl Style = "oneBowl"
	// StyleCustom is the diner's own counts: any number of protein, vegetable
	// and soup dishes, including none of a kind, as long as the meal has one.
	StyleCustom Style = "custom"
)

// Counts is a custom meal's slot counts.
type Counts struct {
	Mains int
	Vegs  int
	Soups int
}

// Total is how many dishes the counts ask for.
func (c Counts) Total() int { return c.Mains + c.Vegs + c.Soups }

// Label is the style's Chinese name.
func (s Style) Label() string {
	switch s {
	case StyleOneBowl:
		return "一碗端"
	case StyleCustom:
		return "自由指定"
	}
	return "两菜一汤"
}

// Draft is a new dish the AI proposed but the library does not have. The
// offline planner never creates one; it exists so an AI answer can be shown
// and validated instead of crashing the parse.
type Draft struct {
	Name            string            `json:"name"`
	Cuisine         string            `json:"cuisine"`
	MainCategory    string            `json:"mainCategory"`
	Intro           string            `json:"intro"`
	Difficulty      string            `json:"difficulty"`
	BaseServings    int               `json:"baseServings"`
	DurationMinutes int               `json:"durationMinutes"`
	Ingredients     []DraftIngredient `json:"ingredients"`
	Steps           []DraftStep       `json:"steps"`
}

// DraftIngredient is one line of a proposed dish.
type DraftIngredient struct {
	Name       string   `json:"name"`
	Amount     float64  `json:"amount"`
	Unit       string   `json:"unit"`
	AmountText string   `json:"amountText"`
	Grams      *float64 `json:"grams"`
	Seasoning  bool     `json:"isSeasoning"`
}

// DraftStep is one instruction of a proposed dish.
type DraftStep struct {
	Instruction     string   `json:"instruction"`
	DurationMinutes float64  `json:"durationMinutes"`
	Resources       []string `json:"resources"`
}

// IngredientNames lists the draft's ingredient names, seasonings included.
func (d Draft) IngredientNames() []string {
	out := make([]string, 0, len(d.Ingredients))
	for _, in := range d.Ingredients {
		if name := strings.TrimSpace(in.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// Dish is one dish of a plan: a library recipe or, for an AI answer, a draft.
type Dish struct {
	RecipeID        int64  `json:"recipeId,omitempty"`
	Name            string `json:"name"`
	Brief           string `json:"brief,omitempty"`
	DurationMinutes int    `json:"durationMinutes,omitempty"`
	WaitMinutes     int    `json:"waitMinutes,omitempty"`
	Kcal            int    `json:"kcal,omitempty"`
	ProteinG        int    `json:"proteinG,omitempty"`
	Slot            Slot   `json:"slot,omitempty"`
	VersionGroup    string `json:"versionGroup,omitempty"`
	// ServeNote is one line about how to share this dish when several people
	// with different limits eat together.
	ServeNote string `json:"serveNote,omitempty"`
	Draft     *Draft `json:"draft,omitempty"`
}

// IsDraft reports whether the dish is not in the library.
func (d Dish) IsDraft() bool { return d.RecipeID == 0 }

// Meal is one meal of the plan.
type Meal struct {
	Title  string `json:"title"`
	Style  Style  `json:"mealStyle,omitempty"`
	Dishes []Dish `json:"dishes"`
}

// MealPlan is a whole answer: one meal per selected meal slot.
type MealPlan struct {
	Meals []Meal `json:"meals"`
	Note  string `json:"note,omitempty"`
}

// PlanInput is everything the planner is allowed to look at. The app fills it
// from its loaded snapshots; nothing here touches a database or a view.
type PlanInput struct {
	// Recipes is the whole library; the pool is filtered from it.
	Recipes []domain.Recipe
	// Stock is the normalized pantry name set used for the "ingredients on
	// hand" preference. Nil means no pantry information.
	Stock map[string]bool
	// Favorites and Cooked drive the safe-vs-explore split. Cooked counts
	// meals in the last 30 days; LastCooked is the last time (any age).
	Favorites  map[int64]bool
	Cooked     map[int64]int
	LastCooked map[int64]time.Time
	// ExcludeIDs and ExcludeGroups drop dishes the last batch already used,
	// so "换一批" really changes the dishes.
	ExcludeIDs    map[int64]bool
	ExcludeGroups map[string]bool

	// Now is the reference moment for season and freshness rules.
	Now time.Time
	// Style is the meal structure; Meals holds one display title per meal.
	Style    Style
	Meals    []string
	Servings int
	// Counts is read when Style is StyleCustom: how many protein, vegetable
	// and soup dishes each meal carries.
	Counts Counts
	// MaxMinutes caps one dish's total time; 0 means no limit.
	MaxMinutes int

	Profile       domain.SichuProfile
	Questionnaire domain.UserProfile
	Diners        []domain.MemberProfile

	// Catalog supplies the per-dish energy estimate. Nil leaves it at zero.
	Catalog *nutrition.Catalog
	// ExploreRatio is 0..1; 0 falls back to the profile's saved ratio.
	ExploreRatio float64
}

// ExploreRatioOrDefault is the effective explore share.
func (in PlanInput) ExploreRatioOrDefault() float64 {
	r := in.ExploreRatio
	if r < 0 {
		r = in.Profile.ExploreRatio
	}
	if r < 0 || r > 1 {
		r = 0.3
	}
	return r
}

// Preferences folds the input's profiles into the limit set.
func (in PlanInput) Preferences() Preferences {
	return PreferencesFrom(in.Profile, in.Questionnaire, in.Diners)
}

// normalize fills the defaults MealPlan needs.
func (in PlanInput) normalize() PlanInput {
	if in.Style == "" {
		in.Style = StyleCombo
	}
	if len(in.Meals) == 0 {
		in.Meals = MealTitles(DefaultMealSlots)
	}
	if in.Servings <= 0 {
		in.Servings = 2
	}
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	return in
}

// candidate is one library dish that survived the hard filters.
type candidate struct {
	rec      domain.Recipe
	shape    Shape
	group    string
	duration int
	stock    int
	favorite bool
	cooked   int
	seasonal bool
	score    int
}

// pool holds the filtered candidates, already ordered the way the planner
// tries them: best score first, then recipe id for a stable tie-break.
type pool struct {
	main    []candidate
	veg     []candidate
	soup    []candidate
	core    []candidate
	ordered []candidate
}

func (p *pool) list(slot Slot) []candidate {
	switch slot {
	case SlotMain:
		return p.main
	case SlotVeg:
		return p.veg
	case SlotSoup:
		return p.soup
	case SlotCore:
		return p.core
	default:
		return p.ordered
	}
}

// buildPool filters the library by the hard limits and orders each slot.
func buildPool(in PlanInput, prefs Preferences) *pool {
	p := &pool{}
	for _, rec := range in.Recipes {
		if rec.ID == 0 || !rec.IsDish {
			continue
		}
		if in.ExcludeIDs[rec.ID] {
			continue
		}
		group := VersionGroup(rec)
		if group != "" && in.ExcludeGroups[group] {
			continue
		}
		ingredients := RecipeIngredientNames(rec)
		if reason := prefs.DishViolation(rec.Name, ingredients); reason != "" {
			continue
		}
		shape := ShapeOf(rec)
		c := candidate{rec: rec, shape: shape, group: group}
		if rec.DurationMinutes != nil {
			c.duration = int(math.Round(*rec.DurationMinutes))
		}
		if in.MaxMinutes > 0 && c.duration > in.MaxMinutes {
			continue
		}
		c.stock, c.favorite, c.cooked, c.seasonal = in.signals(rec)
		c.score = in.score(c, prefs)
		p.ordered = append(p.ordered, c)
		if !shape.Soup && shape.Role == RoleProtein {
			p.main = append(p.main, c)
		}
		if !shape.Soup && shape.Role == RoleVeg {
			p.veg = append(p.veg, c)
		}
		if shape.Soup {
			p.soup = append(p.soup, c)
		}
		if shape.OneBowl && !shape.Soup {
			p.core = append(p.core, c)
		}
	}
	for _, list := range [][]candidate{p.main, p.veg, p.soup, p.core, p.ordered} {
		sortCandidates(list)
	}
	return p
}

func sortCandidates(list []candidate) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].rec.ID < list[j].rec.ID
	})
}

// signals reads the per-dish preference inputs.
func (in PlanInput) signals(rec domain.Recipe) (stock int, favorite bool, cooked int, seasonal bool) {
	seasonal = true
	if len(rec.SeasonMonths) > 0 {
		seasonal = false
		for _, m := range rec.SeasonMonths {
			if m == int(in.Now.Month()) {
				seasonal = true
				break
			}
		}
	}
	favorite = in.Favorites[rec.ID]
	cooked = in.Cooked[rec.ID]
	if in.Stock != nil {
		for _, ing := range rec.Ingredients {
			if !ing.Required {
				continue
			}
			if in.Stock[strings.ToLower(strings.TrimSpace(ing.Name))] {
				stock++
			}
		}
	}
	return
}

// score ranks one dish for this user and this day.
func (in PlanInput) score(c candidate, prefs Preferences) int {
	s := 0
	if c.seasonal {
		s += 4
	}
	if c.stock > 3 {
		s += 9
	} else {
		s += c.stock * 3
	}
	if c.favorite {
		s += 3
	}
	if c.cooked > 0 {
		s += 2
		if c.cooked > 3 {
			s -= 2
		}
	}
	if last, ok := in.LastCooked[c.rec.ID]; ok && in.Now.Sub(last) < 7*24*time.Hour {
		s -= 6
	}
	spicy := spicyLevel(c.rec.Name)
	switch prefs.Spicy {
	case "none":
		if spicy > 0 {
			s -= 8
		}
	case "mild":
		if spicy >= 2 {
			s -= 4
		} else if spicy == 1 {
			s += 2
		}
	case "medium":
		if spicy > 0 {
			s += 2
		}
		if spicy >= 3 {
			s -= 2
		}
	case "hot":
		if spicy > 0 {
			s += 4
		}
	}
	if prefs.Salt == "light" && saltyName(c.rec.Name) {
		s -= 4
	}
	if prefs.Salt == "heavy" && saltyName(c.rec.Name) {
		s += 2
	}
	if prefs.hasGoal("fatLoss") || prefs.hasGoal("sugarControl") {
		if hasAnyWord(c.rec.Name, []string{"蒸", "焯", "凉拌", "白灼", "清炒"}) {
			s += 3
		}
		if hasAnyWord(c.rec.Name, []string{"炸", "酥", "红烧", "糖醋", "拔丝", "油焖"}) {
			s -= 3
		}
	}
	if prefs.hasGoal("muscleGain") && c.shape.Role == RoleProtein {
		s += 2
	}
	if prefs.hasGoal("lowPurine") &&
		hasAnyWord(c.rec.Name, []string{"内脏", "浓汤", "虾", "蟹", "贝"}) {
		s -= 4
	}
	d := c.duration
	if d <= 0 {
		d = 30
	}
	if d > 60 {
		s -= (d - 60) / 20
	}
	return s
}

// usedSet tracks what one plan already ate.
type usedSet struct {
	ids    map[int64]bool
	groups map[string]bool
	names  map[string]bool
}

func newUsedSet() *usedSet {
	return &usedSet{
		ids:    make(map[int64]bool),
		groups: make(map[string]bool),
		names:  make(map[string]bool),
	}
}

func (u *usedSet) has(c candidate) bool {
	if c.rec.ID != 0 && u.ids[c.rec.ID] {
		return true
	}
	if c.group != "" && u.groups[c.group] {
		return true
	}
	return u.names[NormalizeVersionGroup(c.rec.Name)]
}

func (u *usedSet) add(c candidate) {
	if c.rec.ID != 0 {
		u.ids[c.rec.ID] = true
	}
	if c.group != "" {
		u.groups[c.group] = true
	}
	u.names[NormalizeVersionGroup(c.rec.Name)] = true
}

// MealPlan builds one meal per selected meal slot. It is deterministic: no
// randomness, and ties break on recipe id.
func Plan(in PlanInput) (MealPlan, error) {
	in = in.normalize()
	prefs := in.Preferences()
	p := buildPool(in, prefs)

	plan := MealPlan{}
	used := newUsedSet()
	slotsPerMeal := 3
	switch in.Style {
	case StyleOneBowl:
		slotsPerMeal = 2
	case StyleCustom:
		slotsPerMeal = in.Counts.Total()
	}
	totalSlots := len(in.Meals) * slotsPerMeal
	exploreSlots := int(math.Round(float64(totalSlots) * in.ExploreRatioOrDefault()))
	if exploreSlots > totalSlots {
		exploreSlots = totalSlots
	}

	slotIndex := 0
	nextExplore := func() bool {
		explore := slotIndex < exploreSlots
		slotIndex++
		return explore
	}

	for _, title := range in.Meals {
		meal := Meal{Title: title, Style: in.Style}
		if in.Style == StyleOneBowl {
			if pick(&meal, p.core, SlotCore, nextExplore(), used) == nil {
				return MealPlan{}, fmt.Errorf("库里没有合规的一碗端菜式，先去菜式页补几道")
			}
			// A soup is optional for a one-bowl meal; pick appends one when
			// the library has a legal soup left.
			pick(&meal, p.soup, SlotSoup, nextExplore(), used)
			plan.Meals = append(plan.Meals, meal)
			continue
		}
		if in.Style == StyleCustom {
			if in.Counts.Total() == 0 {
				return MealPlan{}, fmt.Errorf("自由配餐至少要选一道菜")
			}
			for i := 0; i < in.Counts.Mains; i++ {
				if pick(&meal, p.main, SlotMain, nextExplore(), used) == nil {
					return MealPlan{}, fmt.Errorf("没有合规的荤菜，检查忌口与过敏设置")
				}
			}
			for i := 0; i < in.Counts.Vegs; i++ {
				if pick(&meal, p.veg, SlotVeg, nextExplore(), used) == nil {
					return MealPlan{}, fmt.Errorf("没有合规的蔬菜类菜，检查忌口与过敏设置")
				}
			}
			for i := 0; i < in.Counts.Soups; i++ {
				if pick(&meal, p.soup, SlotSoup, nextExplore(), used) == nil {
					return MealPlan{}, fmt.Errorf("没有合规的汤羹，先去菜式页补几道")
				}
			}
			plan.Meals = append(plan.Meals, meal)
			continue
		}
		if pick(&meal, p.main, SlotMain, nextExplore(), used) == nil {
			return MealPlan{}, fmt.Errorf("没有合规的荤菜，检查忌口与过敏设置")
		}
		if pick(&meal, p.veg, SlotVeg, nextExplore(), used) == nil {
			return MealPlan{}, fmt.Errorf("没有合规的蔬菜类菜，检查忌口与过敏设置")
		}
		if pick(&meal, p.soup, SlotSoup, nextExplore(), used) == nil {
			return MealPlan{}, fmt.Errorf("没有合规的汤羹，先去菜式页补几道")
		}
		plan.Meals = append(plan.Meals, meal)
	}
	plan.Note = localNote(in, plan, exploreSlots > 0)
	// Fill the per-dish duration, slots and energy estimate before the
	// contract check, so callers get a plan ready to show.
	plan = Enrich(plan, in)
	if err := Validate(plan, in); err != nil {
		return MealPlan{}, err
	}
	return plan, nil
}

// pick appends the best unused candidate to the meal and reports whether it
// found one. Explore slots prefer a dish the user has not cooked; safe slots
// prefer a familiar one. When the preferred side is empty the other side is
// used, so a user without history still gets a plan.
func pick(meal *Meal, list []candidate, slot Slot, explore bool, used *usedSet) *candidate {
	c := pickFrom(list, explore, used)
	if c == nil {
		return nil
	}
	used.add(*c)
	meal.Dishes = append(meal.Dishes, candidateDish(*c, slot))
	return c
}

func pickFrom(list []candidate, explore bool, used *usedSet) *candidate {
	for i := range list {
		c := &list[i]
		if used.has(*c) {
			continue
		}
		familiar := c.favorite || c.cooked > 0
		if explore && familiar {
			continue
		}
		if !explore && !familiar {
			continue
		}
		return c
	}
	for i := range list {
		if !used.has(list[i]) {
			return &list[i]
		}
	}
	return nil
}

// candidateDish turns one chosen candidate into a plan dish.
func candidateDish(c candidate, slot Slot) Dish {
	d := Dish{
		RecipeID:        c.rec.ID,
		Name:            c.rec.Name,
		Slot:            slot,
		VersionGroup:    c.group,
		DurationMinutes: c.duration,
		Brief:           briefOf(c),
	}
	return d
}

// briefOf writes the one-line "why this dish" reason.
func briefOf(c candidate) string {
	parts := make([]string, 0, 3)
	if c.stock >= 2 {
		parts = append(parts, "食材在库")
	}
	if c.seasonal && len(c.rec.SeasonMonths) > 0 && len(c.rec.SeasonMonths) < 12 {
		parts = append(parts, "当季")
	}
	switch {
	case c.favorite:
		parts = append(parts, "你收藏过")
	case c.cooked > 0:
		parts = append(parts, "你做过")
	}
	if len(parts) == 0 {
		if c.duration > 0 && c.duration <= 25 {
			parts = append(parts, "二十来分钟能上桌")
		} else {
			parts = append(parts, "家常稳妥")
		}
	}
	return strings.Join(parts, "，")
}

// localNote explains what the offline planner did, in one short sentence.
func localNote(in PlanInput, plan MealPlan, explored bool) string {
	n := 0
	for _, m := range plan.Meals {
		n += len(m.Dishes)
	}
	note := fmt.Sprintf("本地策略：%d 餐 %d 道，热量与蛋白是估算。", len(plan.Meals), n)
	if explored {
		note += " 按探索比例留了新鲜位。"
	}
	return note
}

// Replace swaps one dish for the next legal alternative in the same slot.
func Replace(in PlanInput, plan MealPlan, mealIndex, dishIndex int) (MealPlan, error) {
	in = in.normalize()
	if mealIndex < 0 || mealIndex >= len(plan.Meals) {
		return plan, errors.New("没有这一餐")
	}
	meal := plan.Meals[mealIndex]
	if dishIndex < 0 || dishIndex >= len(meal.Dishes) {
		return plan, errors.New("没有这道菜")
	}
	prefs := in.Preferences()
	p := buildPool(in, prefs)
	slot := meal.Dishes[dishIndex].Slot
	if slot == "" {
		slot = slotOfDish(meal.Dishes[dishIndex], in)
	}
	used := usedFromPlan(plan, in)
	next := pickFrom(p.list(slot), false, used)
	if next == nil {
		return plan, errors.New("没有更多合规的替换菜")
	}
	nextPlan := clonePlan(plan)
	nextPlan.Meals[mealIndex].Dishes[dishIndex] = candidateDish(*next, slot)
	nextPlan.Note = localNote(in, nextPlan, false)
	nextPlan = Enrich(nextPlan, in)
	if err := Validate(nextPlan, in); err != nil {
		return plan, err
	}
	return nextPlan, nil
}

// usedFromPlan collects everything the plan already contains, so a
// replacement cannot repeat a dish.
func usedFromPlan(plan MealPlan, in PlanInput) *usedSet {
	byID := recipeIndex(in.Recipes)
	used := newUsedSet()
	for _, meal := range plan.Meals {
		for _, dish := range meal.Dishes {
			used.names[NormalizeVersionGroup(dish.Name)] = true
			if dish.RecipeID != 0 {
				used.ids[dish.RecipeID] = true
				if rec, ok := byID[dish.RecipeID]; ok {
					used.groups[VersionGroup(rec)] = true
				}
			} else if dish.VersionGroup != "" {
				used.groups[dish.VersionGroup] = true
			}
		}
	}
	return used
}

func clonePlan(plan MealPlan) MealPlan {
	out := MealPlan{Note: plan.Note, Meals: make([]Meal, len(plan.Meals))}
	for i, m := range plan.Meals {
		out.Meals[i] = Meal{Title: m.Title, Style: m.Style, Dishes: append([]Dish(nil), m.Dishes...)}
	}
	return out
}

// slotOfDish derives a dish's slot from its shape.
func slotOfDish(dish Dish, in PlanInput) Slot {
	shape := dishShape(dish, recipeIndex(in.Recipes))
	switch {
	case shape.Soup:
		return SlotSoup
	case shape.OneBowl:
		return SlotCore
	case shape.Role == RoleVeg:
		return SlotVeg
	default:
		return SlotMain
	}
}

// dishShape derives a dish's structure from the library record or, for a
// draft, from the draft's own category and cooking method.
func dishShape(dish Dish, byID map[int64]domain.Recipe) Shape {
	if dish.RecipeID != 0 {
		if rec, ok := byID[dish.RecipeID]; ok {
			return ShapeOf(rec)
		}
	}
	if dish.Draft != nil {
		return Shape{
			Role:    RoleOfCategory(dish.Draft.MainCategory),
			Soup:    strings.TrimSpace(dish.Draft.Cuisine) == soupCuisine,
			OneBowl: IsOneBowlName(dish.Name),
		}
	}
	return Shape{Role: RoleUnknown, OneBowl: IsOneBowlName(dish.Name)}
}

// Enrich fills a parsed plan with the library's names, durations, slots and
// the catalog's energy estimate. An AI answer carries ids and little else.
func Enrich(plan MealPlan, in PlanInput) MealPlan {
	in = in.normalize()
	byID := recipeIndex(in.Recipes)
	out := clonePlan(plan)
	for mi := range out.Meals {
		meal := &out.Meals[mi]
		if meal.Style == "" {
			meal.Style = in.Style
		}
		for di := range meal.Dishes {
			dish := &meal.Dishes[di]
			if dish.RecipeID != 0 {
				rec, ok := byID[dish.RecipeID]
				if !ok {
					continue
				}
				if strings.TrimSpace(dish.Name) == "" {
					dish.Name = rec.Name
				}
				dish.VersionGroup = VersionGroup(rec)
				if dish.DurationMinutes == 0 && rec.DurationMinutes != nil {
					dish.DurationMinutes = int(math.Round(*rec.DurationMinutes))
				}
				if dish.Kcal == 0 && dish.ProteinG == 0 && in.Catalog != nil {
					n := in.Catalog.Recipe(rec)
					if n.Kcal > 0 {
						dish.Kcal = int(math.Round(n.Kcal))
					}
					if n.ProteinG > 0 {
						dish.ProteinG = int(math.Round(n.ProteinG))
					}
				}
			} else if dish.Draft != nil {
				d := dish.Draft
				if strings.TrimSpace(dish.Name) == "" {
					dish.Name = d.Name
				}
				if dish.DurationMinutes == 0 {
					dish.DurationMinutes = d.DurationMinutes
				}
				dish.VersionGroup = NormalizeVersionGroup(dish.Name)
			}
			if dish.Slot == "" {
				dish.Slot = slotOfDish(*dish, in)
			}
		}
	}
	return out
}

// LegalRecipes returns the library dishes the local layer accepts, ordered the
// way the planner would try them. The AI prompt builds its citable id list
// from this so the model can only choose safe dishes.
func LegalRecipes(in PlanInput) []domain.Recipe {
	in = in.normalize()
	p := buildPool(in, in.Preferences())
	out := make([]domain.Recipe, 0, len(p.ordered))
	for _, c := range p.ordered {
		out = append(out, c.rec)
	}
	return out
}

// RecipeIngredientNames lists a library recipe's ingredient names, seasonings
// included: an allergen inside a sauce still counts.
func RecipeIngredientNames(rec domain.Recipe) []string {
	out := make([]string, 0, len(rec.Ingredients))
	for _, in := range rec.Ingredients {
		if name := strings.TrimSpace(in.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// recipeIndex maps id to recipe.
func recipeIndex(recipes []domain.Recipe) map[int64]domain.Recipe {
	out := make(map[int64]domain.Recipe, len(recipes))
	for _, r := range recipes {
		out[r.ID] = r
	}
	return out
}

// spicyLevel estimates how hot a dish name reads: 0 none, 1 mild, 2 hot,
// 3 very hot.
func spicyLevel(name string) int {
	switch {
	case hasAnyWord(name, []string{"重辣", "特辣", "爆辣", "变态辣"}):
		return 3
	case hasAnyWord(name, []string{"麻辣", "香辣", "辣椒", "剁椒", "泡椒", "小米辣", "花椒", "芥末"}):
		return 2
	case hasAnyWord(name, []string{"辣", "麻"}):
		return 1
	}
	return 0
}

// saltyName reports whether a dish reads as salty or strongly seasoned.
func saltyName(name string) bool {
	return hasAnyWord(name, []string{
		"咸", "酱", "腊", "腌", "卤", "豆豉", "腐乳", "咸菜", "榨菜",
	})
}

// hasAnyWord reports whether s contains any of the words.
func hasAnyWord(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
