package app

// M5 · 私厨与 AI。
//
// This file is the private-kitchen workstream's home: the planner state, the
// body/member profile pages, the result page and the AI key/quota settings.
// sichuPage (the entry page) lives in sichu_views.go.
//
// The pages only read the state below and call the handlers; DB reads and the
// planner itself run through sichuRun, which is a goroutine in the app and an
// inline call in tests without a window.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
	"github.com/gij072858-cmd/foodbox-go/internal/services/ai"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
	"github.com/gij072858-cmd/foodbox-go/internal/services/nutrition"
	"github.com/gij072858-cmd/foodbox-go/internal/services/schedule"
	"github.com/gij072858-cmd/foodbox-go/internal/services/sichu"
)

// Workbench choices. The stock policy is an app-layer approximation: the
// service layer only knows a pantry name set.
const (
	sichuEngineLocal = iota
	sichuEngineAI
)

const (
	sichuStockStrict = iota
	sichuStockExpiring
	sichuStockAllow
)

// sichuDishKey addresses one dish slot in the current plan.
type sichuDishKey struct {
	Meal int
	Dish int
}

// sichuDashSig is the cheap signature of the facts the dashboard reads.
type sichuDashSig struct {
	Items, Records, RecordCount int
	CanCook, Almost, FarAway    int
	FirstID, LastID             int64
	LastAt                      int64
	PantryOff                   bool
}

// sichuDashboard is the right column's pantry summary, rebuilt off the view
// only when its inputs change.
type sichuDashboard struct {
	Ready            bool
	PageAt           time.Time
	Sig              sichuDashSig
	FullStocked      int
	Expiring         []domain.Item
	ExpiringNames    []string
	NutritionVerdict string
	NutritionHint    string
}

// sichuState is the private kitchen's view state.
type sichuState struct {
	// loaded is true once the profile, members and AI status were read.
	loaded bool
	// generating guards the planner/AI job.
	generating bool

	// profile is the body profile; members mirrors member_profiles (self
	// first). questionnaire supplies taste preferences and fallback limits.
	profile       domain.SichuProfile
	questionnaire domain.UserProfile
	members       []domain.MemberProfile
	catalog       *nutrition.Catalog

	// Entry form: structure, selected meals, servings and whose limits apply.
	// style is the mode segment: 0 按人数智能配比, 1 一人食简餐, 2 自由指定菜数.
	// mains/vegs/soups are the custom counts; the smart mode reads its own.
	style       int
	mains       int
	vegs        int
	soups       int
	meals       map[string]bool
	servings    float64
	guests      int      // 临时客, added to the serving count
	whoID       int64    // 0 = the body profile, otherwise a member id
	limitLabels []string // the active limits, rebuilt off the view

	// Workbench choices.
	engine        int // sichuEngineLocal / sichuEngineAI
	engineNotice  string
	stockPolicy   int // sichuStockStrict / Expiring / Allow
	avoidRecent3d bool
	locked        map[sichuDishKey]sichu.Dish

	// Right-column caches; filled off the view.
	dash         sichuDashboard
	planSummary  string
	planKcal     int
	planProtein  int
	planMinutes  int
	planServings int
	planStyle    int
	showBoard    bool // result card's link back to the pantry board

	// Last generated plan.
	plan         sichu.MealPlan
	planReady    bool
	planSource   string
	planError    string
	resultNotice string
	// pushed is true once this plan's meals are in 掌序, so pressing the
	// button again cannot enqueue a meal that was already settled.
	pushed bool

	// Body page draft, edited in place and saved in one transaction.
	body       bodyDraft
	bodyNotice string

	// Member editor and delete confirmation.
	memberEdit   memberDraft
	memberOpen   bool
	memberIsNew  bool
	memberNotice string
	memberDelete int64
	confirmOpen  bool
}

func (s *sichuState) init() {
	s.meals = map[string]bool{"lunch": true, "dinner": true}
	s.servings = 2
	s.guests = 0
	s.mains, s.vegs, s.soups = 1, 1, 1
	s.engine = sichuEngineLocal
	s.stockPolicy = sichuStockAllow
	s.locked = map[sichuDishKey]sichu.Dish{}
	s.body = bodyDraftFrom(domain.SichuProfile{ExploreRatio: 0.3})
}

// aiState is the AI provider's view state: whether a key is configured, the
// remaining quota and the last error. Secrets never reach the view.
type aiState struct {
	loaded     bool
	configured bool
	usage      ai.Usage
	remaining  int
	keyInput   string
	busy       bool
	notice     string
	lastError  string
}

func (s *aiState) init() {
	s.usage = ai.Usage{}.ForDay(ai.Today(time.Now()))
	s.remaining = s.usage.Remaining()
}

// bodyDraft is the editable body form. Numeric fields are float64 because
// MyGo's NumberInput and Slider bind float pointers; enum fields hold the
// Chinese label and are converted back on save.
type bodyDraft struct {
	height       float64
	weight       float64
	age          float64
	targetWeight float64
	bodyFat      float64
	sex          string
	activity     string
	lifeStage    string
	allergens    map[string]bool
	health       map[string]bool
	goals        map[string]bool
	halal        bool
	vegetarian   bool
	allergy      bool
	allergyNote  string
	healthNote   string
	explore      float64
}

// memberDraft is one member being added or edited.
type memberDraft struct {
	id        int64
	name      string
	role      string
	age       float64
	height    float64
	weight    float64
	health    map[string]bool
	allergens map[string]bool
	goals     map[string]bool
}

// memberRoleOptions mirrors the Flutter member role enum names.
var memberRoleOptions = []sichu.Option{
	{Name: "adult", Label: "成人"},
	{Name: "kid", Label: "小孩"},
	{Name: "elder", Label: "老人"},
	{Name: "teen", Label: "青少年"},
	{Name: "pregnant", Label: "孕产妇"},
}

// unchosenLabel is the select entry for "not filled".
const unchosenLabel = "未填"

// loadSichuState reads the body profile, members and AI status. It runs on
// bootstrap's goroutine and publishes state through a.post.
func (a *App) loadSichuState(ctx context.Context, store repository.Repository) error {
	profile, _, err := store.GetSichuProfile(ctx)
	if err != nil {
		return err
	}
	members, err := store.ListMembers(ctx)
	if err != nil {
		return err
	}
	questionnaire, err := store.GetUserProfile(ctx)
	if err != nil {
		return err
	}
	raw, err := store.GetAIUsageJSON(ctx)
	if err != nil {
		return err
	}
	_, configured, keyErr := ai.NewKeyStore().Get()
	catalog, catErr := nutrition.NewCatalog()
	if catErr != nil {
		catalog = nil
	}
	usage := ai.ParseUsage(raw).ForDay(ai.Today(time.Now()))
	a.post(func() {
		a.sichu.loaded = true
		a.sichu.questionnaire = questionnaire
		a.sichu.members = members
		a.sichu.catalog = catalog
		a.setSichuProfile(profile)
		a.ai.loaded = true
		a.ai.configured = configured
		a.ai.usage = usage
		a.ai.remaining = usage.Remaining()
		if keyErr != nil {
			a.ai.lastError = keyErr.Error()
		}
	})
	return nil
}

// setSichuProfile publishes a profile and rebuilds the body draft from it.
func (a *App) setSichuProfile(p domain.SichuProfile) {
	p = sichu.NormalizeProfile(p)
	a.sichu.profile = p
	a.sichu.body = bodyDraftFrom(p)
	a.refreshSichuLimits()
}

// refreshSichuLimits rebuilds the one-line limit summary the entry page
// shows. It runs after a profile, member or diner change, never while
// drawing.
func (a *App) refreshSichuLimits() {
	var diners []domain.MemberProfile
	if a.sichu.whoID != 0 {
		for _, m := range a.sichu.members {
			if m.ID == a.sichu.whoID && !m.IsSelf {
				diners = []domain.MemberProfile{m}
				break
			}
		}
	}
	prefs := sichu.PreferencesFrom(a.sichu.profile, a.sichu.questionnaire, diners)
	a.sichu.limitLabels = prefs.Summary()
}

// sichuEffectiveServings is the count the planner and the counters use: the
// page's servings (or one when a member is the diner) plus temporary guests.
func (a *App) sichuEffectiveServings() int {
	base := a.sichuServings()
	if a.sichu.whoID != 0 {
		base = 1
	}
	if base < 1 {
		base = 1
	}
	return base + a.sichu.guests
}

// sichuUseAI resolves the engine switch to a decision and a short fallback
// notice. The notice is empty when the AI path is actually available.
func (a *App) sichuUseAI() (bool, string) {
	if a.sichu.engine != sichuEngineAI {
		return false, ""
	}
	if !a.ai.configured {
		return false, "未配置 DeepSeek 密钥，本次用本地策略。"
	}
	if a.store == nil {
		return false, "AI 暂不可用，本次用本地策略。"
	}
	if a.ai.usage.ForDay(ai.Today(time.Now())).Exhausted() {
		return false, "今日 AI 配额已用完，本次用本地策略。"
	}
	return true, ""
}

// sichuRecentlyCookedIDs is the hard "avoid the last three days" set.
func (a *App) sichuRecentlyCookedIDs(now time.Time) map[int64]bool {
	cutoff := now.Add(-72 * time.Hour)
	out := make(map[int64]bool)
	for _, rec := range a.records {
		if rec.RecipeID != 0 && !rec.CompletedAt.Before(cutoff) {
			out[rec.RecipeID] = true
		}
	}
	return out
}

// sichuStockMap mixes canonical and raw normalized names because the local
// planner scores stock with the raw lowercased ingredient name.
func (a *App) sichuStockMap() map[string]bool {
	out := make(map[string]bool, len(a.items)*2)
	if a.matcher == nil {
		return out
	}
	for _, it := range a.items {
		out[a.matcher.Canonical(it.Name)] = true
		out[strings.ToLower(strings.TrimSpace(it.Name))] = true
	}
	return out
}

// sichuStrictRecipes keeps only dishes whose required ingredients are all on
// hand. It runs when the input is assembled, never while drawing.
func (a *App) sichuStrictRecipes(in sichu.PlanInput) []domain.Recipe {
	if a.matcher == nil {
		return in.Recipes
	}
	if len(in.Stock) == 0 {
		return nil
	}
	out := make([]domain.Recipe, 0, len(in.Recipes))
	for _, rec := range in.Recipes {
		if !rec.IsDish {
			continue
		}
		if a.matcher.MatchWithStock(rec, in.Stock).Status == cookmatch.CanCook {
			out = append(out, rec)
		}
	}
	return out
}

// sichuExpiringRecipes prefers dishes that consume something expiring. When
// the subset cannot cover the requested structure it falls back to the full
// library, because "prefer" must not turn into "never cook".
func (a *App) sichuExpiringRecipes(in sichu.PlanInput) []domain.Recipe {
	if a.matcher == nil {
		return in.Recipes
	}
	expiring := make(map[string]bool)
	for _, it := range a.items {
		if it.NeedsEatingFirst(in.Now.Unix()) {
			expiring[a.matcher.Canonical(it.Name)] = true
		}
	}
	if len(expiring) == 0 {
		return in.Recipes
	}
	out := make([]domain.Recipe, 0, len(in.Recipes))
	for _, rec := range in.Recipes {
		if !rec.IsDish {
			continue
		}
		for _, ing := range rec.Ingredients {
			if ing.Required && expiring[a.matcher.Canonical(ing.Name)] {
				out = append(out, rec)
				break
			}
		}
	}
	if !a.sichuPoolCovers(out, in) {
		return in.Recipes
	}
	return out
}

// sichuPoolCovers is a cheap precheck: can this candidate list fill the
// requested meal structure at all?
func (a *App) sichuPoolCovers(recipes []domain.Recipe, in sichu.PlanInput) bool {
	meals := len(in.Meals)
	if meals == 0 {
		meals = 1
	}
	counts := in.Counts
	if in.Style == sichu.StyleCombo || in.Style == sichu.StyleOneBowl {
		counts = sichu.Counts{Mains: 1, Vegs: 1, Soups: 1}
	}
	var mains, vegs, soups, cores int
	for _, rec := range recipes {
		if !rec.IsDish {
			continue
		}
		shape := sichu.ShapeOf(rec)
		switch {
		case shape.Soup:
			soups++
		case shape.OneBowl:
			cores++
			if shape.Role == sichu.RoleProtein {
				mains++
			} else if shape.Role == sichu.RoleVeg {
				vegs++
			}
		case shape.Role == sichu.RoleProtein:
			mains++
		case shape.Role == sichu.RoleVeg:
			vegs++
		}
	}
	if in.Style == sichu.StyleOneBowl {
		return cores >= meals
	}
	return mains >= counts.Mains*meals && vegs >= counts.Vegs*meals && soups >= counts.Soups*meals
}

// sichuPlanFullyStocked is the post-check for the AI path under 严守库存.
func (a *App) sichuPlanFullyStocked(plan sichu.MealPlan, in sichu.PlanInput) bool {
	if a.matcher == nil {
		return false
	}
	for _, meal := range plan.Meals {
		for _, dish := range meal.Dishes {
			if dish.RecipeID == 0 {
				return false
			}
			rec, ok := a.recipeByID[dish.RecipeID]
			if !ok {
				return false
			}
			if a.matcher.MatchWithStock(rec, in.Stock).Status != cookmatch.CanCook {
				return false
			}
		}
	}
	return true
}

// sichuRun runs work that belongs on a goroutine in the app. Without a
// window (tests) it runs inline, so state settles before the next frame.
func (a *App) sichuRun(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	go fn()
}

// ---- Body profile ------------------------------------------------------

func bodyDraftFrom(p domain.SichuProfile) bodyDraft {
	d := bodyDraft{
		height:       float64(p.HeightCm),
		weight:       float64(p.WeightKg),
		age:          float64(p.Age),
		targetWeight: float64(p.TargetWeightKg),
		bodyFat:      p.BodyFatPct,
		sex:          sichuLabelOrUnchosen(sichu.SexOptions, p.Sex),
		activity:     sichuLabelOrUnchosen(sichu.ActivityOptions, p.ActivityLevel),
		lifeStage:    sichu.LabelOf(sichu.LifeStageOptions, p.LifeStage),
		allergens:    map[string]bool{},
		health:       map[string]bool{},
		goals:        map[string]bool{},
		allergyNote:  p.AllergyNote,
		healthNote:   p.HealthNote,
		explore:      p.ExploreRatio,
	}
	for _, n := range sichu.NormalizeNames(sichu.AllergenOptions, p.Allergens) {
		d.allergens[n] = true
	}
	for _, n := range sichu.NormalizeNames(sichu.HealthTagOptions, p.HealthTags) {
		d.health[n] = true
	}
	for _, g := range sichu.ParseJSONList(p.NutritionGoals) {
		if name := sichu.NameOf(sichu.GoalOptions, g); name != "" {
			d.goals[name] = true
		}
	}
	for _, r := range sichu.NormalizeNames(sichu.RestrictionOptions, p.Restrictions) {
		switch r {
		case "halal":
			d.halal = true
		case "vegetarian":
			d.vegetarian = true
		case "allergy":
			d.allergy = true
		}
	}
	if d.explore < 0 || d.explore > 1 {
		d.explore = 0.3
	}
	return d
}

// sichuLabelOrUnchosen returns the option's label, or 未填 for an empty name.
func sichuLabelOrUnchosen(options []sichu.Option, name string) string {
	if strings.TrimSpace(name) == "" {
		return unchosenLabel
	}
	for _, o := range options {
		if o.Name == name {
			return o.Label
		}
	}
	return unchosenLabel
}

// sichuOptionName resolves a selected label back to its enum name, or "".
func sichuOptionName(options []sichu.Option, label string) string {
	label = strings.TrimSpace(label)
	if label == "" || label == unchosenLabel {
		return ""
	}
	name := sichu.NameOf(options, label)
	for _, o := range options {
		if o.Name == name {
			return name
		}
	}
	return ""
}

// sichuNamesOfList collects the selected option names in table order.
func sichuNamesOfList(options []sichu.Option, selected map[string]bool) []string {
	out := make([]string, 0, len(selected))
	for _, o := range options {
		if selected[o.Name] {
			out = append(out, o.Name)
		}
	}
	return out
}

// profile builds the persisted profile from the draft, keeping the fields the
// body page does not edit (meal slots, calorie target note).
func (d bodyDraft) profile(base domain.SichuProfile) domain.SichuProfile {
	p := base
	p.HeightCm = int(d.height + 0.5)
	p.WeightKg = int(d.weight + 0.5)
	p.Age = int(d.age + 0.5)
	p.TargetWeightKg = int(d.targetWeight + 0.5)
	p.BodyFatPct = d.bodyFat
	p.Sex = sichuOptionName(sichu.SexOptions, d.sex)
	p.ActivityLevel = sichuOptionName(sichu.ActivityOptions, d.activity)
	p.LifeStage = sichuOptionName(sichu.LifeStageOptions, d.lifeStage)
	p.HealthTags = sichu.JoinList(sichuNamesOfList(sichu.HealthTagOptions, d.health))
	p.Allergens = sichu.JoinList(sichuNamesOfList(sichu.AllergenOptions, d.allergens))
	p.NutritionGoals = sichu.JoinJSONList(sichuNamesOfList(sichu.GoalOptions, d.goals))
	restrictions := make([]string, 0, 3)
	if d.halal {
		restrictions = append(restrictions, "halal")
	}
	if d.vegetarian {
		restrictions = append(restrictions, "vegetarian")
	}
	if d.allergy {
		restrictions = append(restrictions, "allergy")
	}
	p.Restrictions = sichu.JoinList(restrictions)
	p.AllergyNote = strings.TrimSpace(d.allergyNote)
	p.HealthNote = strings.TrimSpace(d.healthNote)
	p.ExploreRatio = d.explore
	return p
}

// saveBodyProfile writes the body profile and the self member in one
// transaction, then refreshes the member list.
func (a *App) saveBodyProfile() {
	p := sichu.NormalizeProfile(a.sichu.body.profile(a.sichu.profile))
	a.sichu.profile = p
	a.sichu.bodyNotice = ""
	store := a.store
	if store == nil {
		a.sichu.bodyNotice = "已保存"
		return
	}
	a.sichuRun(func() {
		ctx := context.Background()
		err := store.SaveSichuProfileWithSelf(ctx, p)
		members, listErr := store.ListMembers(ctx)
		a.post(func() {
			if err != nil {
				a.sichu.bodyNotice = "保存失败：" + err.Error()
				return
			}
			if listErr == nil {
				a.sichu.members = members
				a.refreshSichuLimits()
			}
			a.sichu.bodyNotice = "身体档案已保存，本人成员卡已同步"
		})
	})
}

// ---- Members -----------------------------------------------------------

func memberDraftFrom(m domain.MemberProfile) memberDraft {
	d := memberDraft{
		id:        m.ID,
		name:      m.Name,
		role:      sichu.LabelOf(memberRoleOptions, m.Role),
		age:       float64(m.Age),
		height:    float64(m.HeightCm),
		weight:    float64(m.WeightKg),
		health:    map[string]bool{},
		allergens: map[string]bool{},
		goals:     map[string]bool{},
	}
	if d.role == "" {
		d.role = "成人"
	}
	for _, n := range sichu.NormalizeNames(sichu.HealthTagOptions, m.HealthTags) {
		d.health[n] = true
	}
	for _, n := range sichu.NormalizeNames(sichu.AllergenOptions, m.Allergens) {
		d.allergens[n] = true
	}
	for _, g := range sichu.ParseJSONList(m.NutritionGoals) {
		if name := sichu.NameOf(sichu.GoalOptions, g); name != "" {
			d.goals[name] = true
		}
	}
	if g := sichu.NameOf(sichu.GoalOptions, m.NutritionGoal); g != "" && len(d.goals) == 0 {
		d.goals[g] = true
	}
	return d
}

// member builds the persisted member from the draft.
func (d memberDraft) member() domain.MemberProfile {
	goals := sichuNamesOfList(sichu.GoalOptions, d.goals)
	p := domain.MemberProfile{
		ID:             d.id,
		Name:           strings.TrimSpace(d.name),
		Role:           sichu.NameOf(memberRoleOptions, d.role),
		Age:            int(d.age + 0.5),
		HeightCm:       int(d.height + 0.5),
		WeightKg:       int(d.weight + 0.5),
		HealthTags:     sichu.JoinList(sichuNamesOfList(sichu.HealthTagOptions, d.health)),
		Allergens:      sichu.JoinList(sichuNamesOfList(sichu.AllergenOptions, d.allergens)),
		NutritionGoals: sichu.JoinJSONList(goals),
	}
	if len(goals) > 0 {
		p.NutritionGoal = goals[0]
	}
	return p
}

// openMemberEditor opens the add/edit form for one member.
func (a *App) openMemberEditor(m domain.MemberProfile) {
	if m.IsSelf {
		a.router.Push("/sichu/body")
		return
	}
	a.sichu.memberEdit = memberDraftFrom(m)
	a.sichu.memberIsNew = m.ID == 0
	if a.sichu.memberIsNew {
		a.sichu.memberEdit.role = "成人"
	}
	a.sichu.memberOpen = true
	a.sichu.memberNotice = ""
}

// saveMemberEdit writes the member being edited.
func (a *App) saveMemberEdit() {
	m := a.sichu.memberEdit.member()
	if m.Name == "" {
		a.sichu.memberNotice = "请先填写称呼"
		return
	}
	if m.IsSelf {
		a.sichu.memberNotice = "本人卡随身体档案保存"
		return
	}
	store := a.store
	edit := a.sichu.memberEdit
	if store == nil {
		a.sichu.memberOpen = false
		a.sichu.memberNotice = "已保存"
		return
	}
	a.sichuRun(func() {
		ctx := context.Background()
		var err error
		if edit.id == 0 {
			var id int64
			id, err = store.CreateMember(ctx, m)
			if err == nil {
				a.post(func() { a.sichu.whoID = id })
			}
		} else {
			err = store.UpdateMember(ctx, m)
		}
		members, listErr := store.ListMembers(ctx)
		a.post(func() {
			if err != nil {
				a.sichu.memberNotice = "保存失败：" + err.Error()
				return
			}
			if listErr == nil {
				a.sichu.members = members
				a.refreshSichuLimits()
			}
			a.sichu.memberOpen = false
			a.sichu.memberNotice = "已保存 " + m.Name
		})
	})
}

// askDeleteMember confirms then deletes one member.
func (a *App) askDeleteMember(id int64) {
	a.sichu.memberDelete = id
	a.sichu.confirmOpen = true
}

// deleteMember removes one member and clears the diner selection if it was
// that member.
func (a *App) deleteMember(id int64) {
	store := a.store
	if id == 0 {
		return
	}
	if store == nil {
		a.sichu.members = removeMember(a.sichu.members, id)
		if a.sichu.whoID == id {
			a.sichu.whoID = 0
		}
		a.refreshSichuLimits()
		return
	}
	a.sichuRun(func() {
		ctx := context.Background()
		err := store.DeleteMember(ctx, id)
		members, listErr := store.ListMembers(ctx)
		a.post(func() {
			if err != nil {
				a.sichu.memberNotice = "删除失败：" + err.Error()
				return
			}
			if listErr == nil {
				a.sichu.members = members
			}
			if a.sichu.whoID == id {
				a.sichu.whoID = 0
			}
			a.refreshSichuLimits()
			a.sichu.memberNotice = "已删除该成员"
		})
	})
}

func removeMember(members []domain.MemberProfile, id int64) []domain.MemberProfile {
	out := members[:0]
	for _, m := range members {
		if m.ID != id {
			out = append(out, m)
		}
	}
	return out
}

// ---- Planner job -------------------------------------------------------

// buildSichuInput snapshots everything the planner may look at.
func (a *App) buildSichuInput() sichu.PlanInput {
	servings := a.sichuEffectiveServings()
	style := sichu.StyleCustom
	counts := sichu.Counts{Mains: a.sichu.mains, Vegs: a.sichu.vegs, Soups: a.sichu.soups}
	switch a.sichu.style {
	case 1:
		style = sichu.StyleOneBowl
	case 0:
		counts = smartSichuCounts(servings)
	}
	meals := make([]string, 0, 3)
	for _, o := range sichu.MealSlotOptions {
		if a.sichu.meals[o.Name] {
			meals = append(meals, o.Label)
		}
	}
	if len(meals) == 0 {
		meals = sichu.MealTitles(sichu.DefaultMealSlots)
	}
	in := sichu.PlanInput{
		Recipes:       a.recipes,
		Style:         style,
		Counts:        counts,
		Meals:         meals,
		Servings:      servings,
		Now:           time.Now(),
		Profile:       a.sichu.profile,
		Questionnaire: a.sichu.questionnaire,
		Catalog:       a.sichu.catalog,
		ExploreRatio:  a.sichu.profile.ExploreRatio,
	}
	if a.matcher != nil && !a.pantryOff {
		in.Stock = a.sichuStockMap()
	}
	if len(a.favorites) > 0 {
		in.Favorites = a.favorites
	}
	cut := in.Now.AddDate(0, 0, -30)
	cooked := make(map[int64]int)
	last := make(map[int64]time.Time)
	for _, rec := range a.records {
		if !rec.CompletedAt.Before(cut) {
			cooked[rec.RecipeID]++
		}
		if rec.CompletedAt.After(last[rec.RecipeID]) {
			last[rec.RecipeID] = rec.CompletedAt
		}
	}
	if len(cooked) > 0 {
		in.Cooked = cooked
	}
	if len(last) > 0 {
		in.LastCooked = last
	}
	if a.sichu.whoID != 0 {
		for _, m := range a.sichu.members {
			if m.ID == a.sichu.whoID && !m.IsSelf {
				in.Diners = []domain.MemberProfile{m}
				break
			}
		}
	}
	if a.sichu.avoidRecent3d {
		if in.ExcludeIDs == nil {
			in.ExcludeIDs = make(map[int64]bool)
		}
		for id := range a.sichuRecentlyCookedIDs(in.Now) {
			in.ExcludeIDs[id] = true
		}
	}
	if !a.pantryOff {
		switch a.sichu.stockPolicy {
		case sichuStockStrict:
			in.Recipes = a.sichuStrictRecipes(in)
		case sichuStockExpiring:
			in.Recipes = a.sichuExpiringRecipes(in)
		}
	}
	return in
}

// generateSichuPlan plans one batch. It uses the AI provider when the engine
// switch picks it and the key/quota allow it, and falls back to the offline
// plan on any failure. Work runs off the view.
func (a *App) generateSichuPlan(regenerate bool) {
	if a.sichu.generating {
		return
	}
	a.sichu.pushed = false
	in := a.buildSichuInput()
	useAI, notice := a.sichuUseAI()
	a.sichu.engineNotice = notice
	strict := !a.pantryOff && a.sichu.stockPolicy == sichuStockStrict
	if strict && !a.sichuPoolCovers(in.Recipes, in) {
		a.sichu.planError = "严守库存下凑不齐这桌，改选允许微缺或先补货"
		return
	}
	if regenerate {
		ids, groups := planExclusion(a.sichu.plan)
		mergeSichuExclusions(&in, ids, groups)
	}
	locked := a.sichuLockSnapshot()
	if len(locked) > 0 {
		ids, groups := a.sichuLockedSets(locked)
		mergeSichuExclusions(&in, ids, groups)
	}
	// The expiring policy narrows the pool. If the narrow pool cannot make a
	// legal meal, retry once against the full library.
	var fallback *sichu.PlanInput
	if a.sichu.stockPolicy == sichuStockExpiring && len(in.Recipes) != len(a.recipes) {
		alt := in
		alt.Recipes = append([]domain.Recipe(nil), a.recipes...)
		fallback = &alt
	}
	// Snapshot the quota before leaving the main thread: the goroutine must
	// not read state another frame may be writing.
	usage := a.ai.usage.ForDay(ai.Today(time.Now()))
	style := a.sichu.style
	a.sichu.generating = true
	a.sichu.showBoard = false
	a.sichu.planError = ""
	a.sichu.resultNotice = ""
	store := a.store
	a.sichuRun(func() {
		plan, source, _, err := a.planSichu(in, fallback, store, useAI, usage, strict)
		a.post(func() {
			a.sichu.generating = false
			if err != nil {
				a.sichu.planError = err.Error()
				a.sichu.planReady = false
				return
			}
			if len(locked) > 0 {
				next, applied, lockErr := a.applySichuLocks(plan, in, locked)
				if lockErr != nil {
					a.clearSichuLocks()
					a.sichu.resultNotice = "锁定菜式与新方案冲突，已放弃锁定"
				} else {
					plan = next
					if applied > 0 {
						a.sichu.resultNotice = fmt.Sprintf("已保留 %d 道锁定菜", applied)
					}
				}
			}
			if useAI && source == "本地" && a.sichu.engineNotice == "" {
				a.sichu.engineNotice = "AI 未返回可用方案，已用本地策略。"
			}
			a.sichu.plan = plan
			a.sichu.planReady = true
			a.sichu.planSource = source
			a.sichu.planStyle = style
			a.refreshSichuPlanDerived()
		})
	})
}

// planSichu tries AI first and always ends with a validated plan or an error.
// fallback, when set, is a full-library input used only after the narrowed
// expiring pool has failed.
func (a *App) planSichu(in sichu.PlanInput, fallback *sichu.PlanInput, store repository.Repository, useAI bool, usage ai.Usage, strict bool) (sichu.MealPlan, string, string, error) {
	local := func(src sichu.PlanInput) (sichu.MealPlan, error) {
		plan, err := sichu.Plan(src)
		if err != nil {
			return sichu.MealPlan{}, err
		}
		return sichu.Enrich(plan, src), nil
	}
	var aiErr error
	if useAI {
		if key, ok, err := ai.NewKeyStore().Get(); err == nil && ok {
			provider := ai.NewProvider(key)
			plan, err := provider.Generate(context.Background(), in)
			aiErr = err
			if err == nil {
				next := usage.After()
				if store != nil {
					_ = store.SaveAIUsageJSON(context.Background(), next.JSON())
				}
				a.post(func() {
					a.ai.usage = next
					a.ai.remaining = next.Remaining()
					a.ai.lastError = ""
				})
				if !strict || a.sichuPlanFullyStocked(plan, in) {
					note := strings.TrimSpace(plan.Note)
					if note == "" {
						note = "AI 按档案配的，热量与蛋白是估算。"
					}
					return plan, "AI", note, nil
				}
				aiErr = fmt.Errorf("AI 方案不是全部齐备")
				if localPlan, localErr := local(in); localErr == nil {
					return localPlan, "本地", "AI 方案不是全部齐备，已用本地策略", nil
				}
			} else {
				a.post(func() { a.ai.lastError = sichuClip(aiErr.Error()) })
				if localPlan, localErr := local(in); localErr == nil {
					return localPlan, "本地",
						"AI 暂时不可用，已用本地策略：" + sichuClip(aiErr.Error()), nil
				}
			}
		}
	}
	if plan, err := local(in); err == nil {
		return plan, "本地", plan.Note, nil
	}
	if fallback != nil {
		if plan, err := local(*fallback); err == nil {
			return plan, "本地", "临期食材不足以单独成桌，已按全部库存配", nil
		}
	}
	if aiErr != nil {
		return sichu.MealPlan{}, "", "",
			fmt.Errorf("AI 失败（%s）；本地策略也没配出合规的一桌", sichuClip(aiErr.Error()))
	}
	return sichu.MealPlan{}, "", "", fmt.Errorf("本地策略没配出合规的一桌，检查忌口、库存与菜式数量")
}

// replaceSichuDish swaps one dish in the current plan.
func (a *App) replaceSichuDish(mealIndex, dishIndex int) {
	if !a.sichu.planReady {
		return
	}
	in := a.buildSichuInput()
	next, err := sichu.Replace(in, a.sichu.plan, mealIndex, dishIndex)
	if err != nil {
		a.sichu.resultNotice = err.Error()
		return
	}
	delete(a.sichu.locked, sichuDishKey{Meal: mealIndex, Dish: dishIndex})
	a.sichu.plan = next
	a.sichu.pushed = false
	a.sichu.resultNotice = "已换一道。"
	a.refreshSichuPlanDerived()
}

// mergeSichuExclusions adds ids and groups to an input's exclusion sets.
func mergeSichuExclusions(in *sichu.PlanInput, ids map[int64]bool, groups map[string]bool) {
	if len(ids) > 0 {
		if in.ExcludeIDs == nil {
			in.ExcludeIDs = make(map[int64]bool)
		}
		for id := range ids {
			in.ExcludeIDs[id] = true
		}
	}
	if len(groups) > 0 {
		if in.ExcludeGroups == nil {
			in.ExcludeGroups = make(map[string]bool)
		}
		for g := range groups {
			in.ExcludeGroups[g] = true
		}
	}
}

// sichuLockSnapshot copies the lock map before the planner job leaves the
// main thread.
func (a *App) sichuLockSnapshot() map[sichuDishKey]sichu.Dish {
	out := make(map[sichuDishKey]sichu.Dish, len(a.sichu.locked))
	for k, v := range a.sichu.locked {
		out[k] = v
	}
	return out
}

// sichuLockedSets is the id/version-group set the planner must avoid so a
// locked dish is not duplicated in another slot.
func (a *App) sichuLockedSets(locked map[sichuDishKey]sichu.Dish) (map[int64]bool, map[string]bool) {
	ids := make(map[int64]bool)
	groups := make(map[string]bool)
	for _, dish := range locked {
		if dish.RecipeID != 0 {
			ids[dish.RecipeID] = true
		}
		group := dish.VersionGroup
		if group == "" {
			group = sichu.NormalizeVersionGroup(dish.Name)
		}
		if group != "" {
			groups[group] = true
		}
	}
	return ids, groups
}

// applySichuLocks puts locked dishes back into a freshly generated plan and
// validates the result. Validation needs the full library and no exclusions
// for the locked dishes, or Validate would reject them as "already seen".
func (a *App) applySichuLocks(plan sichu.MealPlan, in sichu.PlanInput, locked map[sichuDishKey]sichu.Dish) (sichu.MealPlan, int, error) {
	work := cloneSichuPlan(plan)
	applied := 0
	for key, dish := range locked {
		if key.Meal < 0 || key.Meal >= len(work.Meals) {
			continue
		}
		if key.Dish < 0 || key.Dish >= len(work.Meals[key.Meal].Dishes) {
			continue
		}
		work.Meals[key.Meal].Dishes[key.Dish] = dish
		applied++
	}
	if applied == 0 {
		return plan, 0, nil
	}
	work = sichu.Enrich(work, in)
	vin := in
	vin.Recipes = append([]domain.Recipe(nil), a.recipes...)
	vin.ExcludeIDs = cloneIDs(in.ExcludeIDs)
	vin.ExcludeGroups = cloneGroups(in.ExcludeGroups)
	for _, dish := range locked {
		if dish.RecipeID != 0 {
			delete(vin.ExcludeIDs, dish.RecipeID)
			if rec, ok := a.recipeByID[dish.RecipeID]; ok {
				delete(vin.ExcludeGroups, sichu.VersionGroup(rec))
			}
		}
		if group := dish.VersionGroup; group != "" {
			delete(vin.ExcludeGroups, group)
		} else {
			delete(vin.ExcludeGroups, sichu.NormalizeVersionGroup(dish.Name))
		}
	}
	if err := sichu.Validate(work, vin); err != nil {
		return plan, 0, err
	}
	return work, applied, nil
}

func cloneIDs(src map[int64]bool) map[int64]bool {
	out := make(map[int64]bool, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneGroups(src map[string]bool) map[string]bool {
	out := make(map[string]bool, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneSichuPlan(plan sichu.MealPlan) sichu.MealPlan {
	out := plan
	out.Meals = make([]sichu.Meal, len(plan.Meals))
	for i, meal := range plan.Meals {
		meal.Dishes = append([]sichu.Dish(nil), meal.Dishes...)
		out.Meals[i] = meal
	}
	return out
}

// toggleSichuLock pins or unpins one dish slot.
func (a *App) toggleSichuLock(mealIndex, dishIndex int, dish sichu.Dish) {
	if a.sichu.locked == nil {
		a.sichu.locked = make(map[sichuDishKey]sichu.Dish)
	}
	key := sichuDishKey{Meal: mealIndex, Dish: dishIndex}
	if _, ok := a.sichu.locked[key]; ok {
		delete(a.sichu.locked, key)
		a.sichu.resultNotice = "已取消锁定「" + dish.Name + "」"
		return
	}
	a.sichu.locked[key] = dish
	a.sichu.resultNotice = "已锁定「" + dish.Name + "」"
}

// sichuLocked reports whether a slot is pinned.
func (a *App) sichuLocked(mealIndex, dishIndex int) bool {
	_, ok := a.sichu.locked[sichuDishKey{Meal: mealIndex, Dish: dishIndex}]
	return ok
}

// clearSichuLocks drops every pin; structural changes call it.
func (a *App) clearSichuLocks() {
	a.sichu.locked = map[sichuDishKey]sichu.Dish{}
}

// planExclusion is the recipe id and version group set a new batch must avoid.
func planExclusion(plan sichu.MealPlan) (map[int64]bool, map[string]bool) {
	ids := make(map[int64]bool)
	groups := make(map[string]bool)
	for _, meal := range plan.Meals {
		for _, dish := range meal.Dishes {
			if dish.RecipeID != 0 {
				ids[dish.RecipeID] = true
			}
			group := dish.VersionGroup
			if group == "" {
				group = sichu.NormalizeVersionGroup(dish.Name)
			}
			if group != "" {
				groups[group] = true
			}
		}
	}
	return ids, groups
}

// addPlanToSchedule queues the plan's library dishes for cooking, keeping
// each meal as its own group. It reports whether anything was pushed, so the
// caller can route to 掌序.
func (a *App) addPlanToSchedule() bool {
	if a.sichu.pushed {
		return true
	}
	items := make([]domain.ScheduleItem, 0, 6)
	drafts := 0
	meals := make([]string, 0, 3)
	for _, meal := range a.sichu.plan.Meals {
		count := 0
		for _, dish := range meal.Dishes {
			if dish.RecipeID == 0 {
				drafts++
				continue
			}
			items = append(items, domain.ScheduleItem{
				RecipeID: dish.RecipeID, Servings: a.sichu.planServings, Meal: meal.Title,
			})
			count++
		}
		if count > 0 {
			meals = append(meals, fmt.Sprintf("%s %d", meal.Title, count))
		}
	}
	if len(items) == 0 {
		a.sichu.resultNotice = "这版都是新菜草案，先在菜式页新建再加入掌序"
		return false
	}
	a.addRecipesToSchedule(items)
	msg := fmt.Sprintf("已加入掌序 %d 道", len(items))
	if len(meals) > 1 {
		msg += "（" + strings.Join(meals, "、") + "）"
	}
	if drafts > 0 {
		msg += fmt.Sprintf("，另有 %d 道新菜暂不入掌序", drafts)
	}
	a.sichu.resultNotice = msg
	a.sichu.pushed = true
	return true
}

// refreshSichuPlanDerived fills the right column's one-line summary and the
// schedule's parallel-time estimate. It runs on the main thread after the
// plan changes.
func (a *App) refreshSichuPlanDerived() {
	a.sichu.planServings = a.sichuEffectiveServings()
	a.sichu.planSummary = sichu.Describe(a.sichu.plan)
	kcal, protein := 0, 0
	for _, meal := range a.sichu.plan.Meals {
		for _, dish := range meal.Dishes {
			kcal += dish.Kcal
			protein += dish.ProteinG
		}
	}
	a.sichu.planKcal = kcal
	a.sichu.planProtein = protein
	recipes := a.sichuPlanRecipes()
	if len(recipes) == 0 || len(a.steps) == 0 {
		a.sichu.planMinutes = 0
		return
	}
	servings := make(map[int64]int, len(recipes))
	for _, rec := range recipes {
		servings[rec.ID] = a.sichu.planServings
	}
	stove := a.sched.stoveCount
	if stove < 1 {
		stove = schedule.DefaultStoveCount
	}
	a.sichu.planMinutes = schedule.BuildTimeline(recipes, a.steps, servings, stove).TotalMinutes
}

// sichuPlanRecipes is the plan's library dishes in plan order; drafts and
// missing ids are skipped because they cannot enter 掌序.
func (a *App) sichuPlanRecipes() []domain.Recipe {
	out := make([]domain.Recipe, 0, 6)
	for _, meal := range a.sichu.plan.Meals {
		for _, dish := range meal.Dishes {
			if dish.RecipeID == 0 {
				continue
			}
			if rec, ok := a.recipeByID[dish.RecipeID]; ok {
				out = append(out, rec)
			}
		}
	}
	return out
}

// ensureSichuDashboard rebuilds the pantry summary only when the page opened
// or one of its cheap inputs changed.
func (a *App) ensureSichuDashboard() {
	sig := a.sichuDashboardSignature()
	if a.sichu.dash.Ready && a.sichu.dash.PageAt.Equal(a.pageAt) && a.sichu.dash.Sig == sig {
		return
	}
	a.refreshSichuDashboard(time.Now())
}

func (a *App) sichuDashboardSignature() sichuDashSig {
	sig := sichuDashSig{
		Items: len(a.items), Records: len(a.records), RecordCount: a.recordCount,
		CanCook: a.canCook, Almost: a.almost, FarAway: a.farAway,
		PantryOff: a.pantryOff,
	}
	if len(a.records) > 0 {
		sig.FirstID = a.records[0].ID
		sig.LastID = a.records[len(a.records)-1].ID
		sig.LastAt = a.records[0].CompletedAt.Unix()
	}
	return sig
}

// refreshSichuDashboard derives the three headline numbers and the one-line
// nutrition hint. It runs off the frame; the view only reads the cache.
func (a *App) refreshSichuDashboard(now time.Time) {
	d := sichuDashboard{
		Ready:       true,
		PageAt:      a.pageAt,
		Sig:         a.sichuDashboardSignature(),
		FullStocked: a.canCook,
	}
	for _, it := range a.items {
		if !it.NeedsEatingFirst(now.Unix()) {
			continue
		}
		d.Expiring = append(d.Expiring, it)
		d.ExpiringNames = append(d.ExpiringNames, it.Name)
	}
	d.NutritionVerdict, d.NutritionHint = a.sichuNutritionHint(now)
	a.sichu.dash = d
}

// sichuNutritionHint is a rule over the last seven days of cooked dishes. It
// is a nudge, not a nutrition conclusion.
func (a *App) sichuNutritionHint(now time.Time) (string, string) {
	cutoff := now.AddDate(0, 0, -7)
	protein, veg, soup, known := 0, 0, 0, 0
	for _, rec := range a.records {
		if rec.CompletedAt.Before(cutoff) {
			continue
		}
		recp, ok := a.recipeByID[rec.RecipeID]
		if !ok {
			continue
		}
		known++
		shape := sichu.ShapeOf(recp)
		if shape.Soup {
			soup++
			continue
		}
		switch shape.Role {
		case sichu.RoleProtein:
			protein++
		case sichu.RoleVeg:
			veg++
		}
	}
	switch {
	case known == 0:
		return "未记录", "近 7 天还没有做饭记录，先按档案配一桌均衡的。"
	case protein == 0:
		return "缺蛋白", "近 7 天蛋白质偏少，今天可以多配一道荤菜或豆制品。"
	case veg == 0:
		return "缺蔬菜", "近 7 天几乎没吃蔬菜，今天补一道绿叶菜。"
	case protein >= veg*2:
		return "荤多素少", "今天用蔬菜和汤找平。"
	case veg >= protein*2:
		return "素多荤少", "今天补一道蛋白质。"
	case soup == 0:
		return "缺汤", "今天可以配一道清汤。"
	default:
		return "大致均衡", "按口味换换花样就好。"
	}
}

// sichuClip shortens a message for one line without cutting a rune.
func sichuClip(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	runes := []rune(s)
	if len(runes) <= 60 {
		return s
	}
	return string(runes[:60]) + "…"
}

// ---- AI settings -------------------------------------------------------

// refreshAIState reloads the key status and quota.
func (a *App) refreshAIState() {
	store := a.store
	a.sichuRun(func() {
		_, configured, keyErr := ai.NewKeyStore().Get()
		var raw string
		if store != nil {
			raw, _ = store.GetAIUsageJSON(context.Background())
		}
		usage := ai.ParseUsage(raw).ForDay(ai.Today(time.Now()))
		a.post(func() {
			a.ai.loaded = true
			a.ai.configured = configured
			a.ai.usage = usage
			a.ai.remaining = usage.Remaining()
			if keyErr != nil {
				a.ai.lastError = keyErr.Error()
			}
		})
	})
}

// saveAIKey stores the pasted key in the system credential store.
func (a *App) saveAIKey() {
	key := strings.TrimSpace(a.ai.keyInput)
	if key == "" {
		a.ai.notice = "请先粘贴密钥"
		return
	}
	store := ai.NewKeyStore()
	a.ai.busy = true
	a.sichuRun(func() {
		err := store.Set(key)
		a.post(func() {
			a.ai.busy = false
			if err != nil {
				a.ai.notice = err.Error()
				return
			}
			a.ai.keyInput = ""
			a.ai.configured = true
			a.sichu.engine = sichuEngineAI
			a.ai.notice = "密钥已保存到本机凭据管理器"
		})
	})
}

// clearAIKey removes the key and today's counter.
func (a *App) clearAIKey() {
	store := ai.NewKeyStore()
	db := a.store
	a.ai.busy = true
	a.sichuRun(func() {
		err := store.Delete()
		if db != nil {
			_ = db.ClearAIUsage(context.Background())
		}
		a.post(func() {
			a.ai.busy = false
			if err != nil {
				a.ai.notice = err.Error()
				return
			}
			a.ai.configured = false
			a.sichu.engine = sichuEngineLocal
			a.ai.usage = ai.Usage{}.ForDay(ai.Today(time.Now()))
			a.ai.remaining = a.ai.usage.Remaining()
			a.ai.notice = "已清除密钥，接下来使用本地策略"
		})
	})
}

// testAIConnection sends one tiny request that does not count against the
// daily quota.
func (a *App) testAIConnection() {
	store := ai.NewKeyStore()
	a.ai.busy = true
	a.ai.notice = "正在测试连接…"
	a.sichuRun(func() {
		key, ok, err := store.Get()
		var notice string
		if err != nil {
			notice = err.Error()
		} else if !ok {
			notice = "还没有配置密钥"
		} else {
			provider := ai.NewProvider(key)
			if pingErr := provider.Ping(context.Background()); pingErr != nil {
				notice = "连接失败：" + sichuClip(pingErr.Error())
			} else {
				notice = "连接正常"
			}
		}
		a.post(func() {
			a.ai.busy = false
			a.ai.notice = notice
		})
	})
}

// ---- Result page -------------------------------------------------------

// sichuResultPage is a compatibility alias: the workbench now shows the plan
// in its right column, so a direct visit lands on the same page.
func (a *App) sichuResultPage(c *ui.Context, p design.Palette) {
	a.sichuPage(c, p)
}

// ---- Body page ---------------------------------------------------------

func (a *App) sichuBodyPage(c *ui.Context, p design.Palette) {
	d := &a.sichu.body
	a.pageBody(c, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.BackButton(c, a.router)
				ui.Text(c, "身体档案").FontSize(12).TextColor(p.Muted)
			})
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "同桌成员").Clicked() {
					a.router.Push("/sichu/members")
				}
				if ui.PrimaryButton(c, "保存身体档案").Clicked() {
					a.saveBodyProfile()
				}
			})
		})
		ui.Text(c, "身体档案是私厨配餐的依据，也是「本人」成员卡的唯一来源；保存时两张表一起写。").
			FontSize(12).TextColor(p.Muted).LineHeight(1.5).Margin(0, 0, design.Sp(c, 4), 0)

		a.section(c, p, "基础数据", func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 3)).Padding(design.Sp(c, 4)).Radius(design.RadiusCard).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				ui.Row(c).Gap(16).Children(func() {
					a.sichuNumberField(c, p, "身高 cm", &d.height, 80, 250, 1)
					a.sichuNumberField(c, p, "体重 kg", &d.weight, 20, 250, 1)
					a.sichuNumberField(c, p, "年龄", &d.age, 1, 120, 1)
				})
				ui.Row(c).Gap(16).Children(func() {
					a.sichuNumberField(c, p, "目标体重 kg", &d.targetWeight, 20, 250, 1)
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						design.FieldLabel(c, p, fmt.Sprintf("体脂 %d%%", int(d.bodyFat+0.5)))
						ui.StepSlider(c, &d.bodyFat, 0, 60, 1).Label("体脂")
					})
				})
				ui.Row(c).Gap(16).Children(func() {
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						design.FieldLabel(c, p, "性别")
						ui.Select(c, &d.sex, []string{unchosenLabel, "男", "女"})
					})
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						design.FieldLabel(c, p, "活动量")
						ui.Select(c, &d.activity, sichuOptionLabels(sichu.ActivityOptions, true))
					})
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						design.FieldLabel(c, p, "生理阶段")
						ui.Select(c, &d.lifeStage, sichuOptionLabels(sichu.LifeStageOptions, false))
					})
				})
				ui.Text(c, "目标体重与体脂可不填，0 视为未填；这些数字只用于估算，不做医学判断。").
					FontSize(11).TextColor(p.Faint)
			})
		})

		a.section(c, p, "忌口与过敏", func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 2.5)).Padding(design.Sp(c, 4)).Radius(design.RadiusCard).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					a.sichuToggleChip(c, p, "chk-halal", "清真", &d.halal)
					a.sichuToggleChip(c, p, "chk-veg", "吃斋", &d.vegetarian)
					a.sichuToggleChip(c, p, "chk-allergy", "过敏", &d.allergy)
				})
				if strings.TrimSpace(a.sichu.profile.Restrictions) == "" &&
					strings.TrimSpace(a.sichu.questionnaire.Restrictions) != "" {
					inherited := sichu.LabelsOf(sichu.RestrictionOptions,
						sichu.EffectiveRestrictions(a.sichu.profile, a.sichu.questionnaire))
					if len(inherited) > 0 {
						ui.Text(c, "当前沿用问卷档案的忌口："+strings.Join(inherited, "、")+
							"；在这里勾选会单独覆盖问卷。").FontSize(11).TextColor(p.Faint)
					}
				}
				design.FieldLabel(c, p, "结构化过敏源")
				ui.Row(c).Gap(6).Wrap().Children(func() {
					for i := range sichu.AllergenOptions {
						o := sichu.AllergenOptions[i]
						a.sichuMapChip(c, p, "aller-"+o.Name, o.Label, o.Name, d.allergens)
					}
				})
				design.FieldLabel(c, p, "自填过敏源")
				ui.TextInput(c, &d.allergyNote).Placeholder("一项一个食材名，用顿号分开")
			})
		})

		a.section(c, p, "健康状况（多选）", func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 2.5)).Padding(design.Sp(c, 4)).Radius(design.RadiusCard).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				ui.Row(c).Gap(6).Wrap().Children(func() {
					for i := range sichu.HealthTagOptions {
						o := sichu.HealthTagOptions[i]
						a.sichuMapChip(c, p, "health-"+o.Name, o.Label, o.Name, d.health)
					}
				})
				ui.Text(c, "忌口与疾病是硬过滤：不合规的菜不会出现在配餐里。").
					FontSize(11).TextColor(p.Faint)
				design.FieldLabel(c, p, "备注")
				ui.TextArea(c, &d.healthNote).Lines(2, 4).Placeholder("例如：少盐、忌辛辣")
			})
		})

		a.section(c, p, "营养目标", func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 2.5)).Padding(design.Sp(c, 4)).Radius(design.RadiusCard).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				ui.Row(c).Gap(6).Wrap().Children(func() {
					for i := range sichu.GoalOptions {
						o := sichu.GoalOptions[i]
						a.sichuMapChip(c, p, "goal-"+o.Name, o.Label, o.Name, d.goals)
					}
				})
				ui.Text(c, "目标只是配餐倾向；热量与蛋白质都是估算值。").FontSize(11).TextColor(p.Faint)
			})
		})

		a.section(c, p, "探索比例", func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 2)).Padding(design.Sp(c, 4)).Radius(design.RadiusCard).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				ui.Text(c, fmt.Sprintf("探索 %d%% · 稳妥 %d%%",
					int(d.explore*100+0.5), 100-int(d.explore*100+0.5))).
					FontSize(13).FontWeight(600).TextColor(p.Text)
				ui.Slider(c, &d.explore, 0, 1)
				ui.Text(c, "比例决定新鲜菜与常吃菜的搭配，是倾向不是硬性配额。").
					FontSize(11).TextColor(p.Faint)
			})
		})

		ui.Row(c).Gap(12).Margin(0, 0, 12, 0).Children(func() {
			if ui.PrimaryButton(c, "保存身体档案").Clicked() {
				a.saveBodyProfile()
			}
			if ui.Button(c, "同桌成员").Clicked() {
				a.router.Push("/sichu/members")
			}
			if a.sichu.bodyNotice != "" {
				ui.Text(c, a.sichu.bodyNotice).FontSize(12).TextColor(p.Accent)
			}
		})
	})
}

// sichuNumberField is a labelled number input bound to a draft field.
func (a *App) sichuNumberField(c *ui.Context, p design.Palette, label string, value *float64, lo, hi, step float64) {
	ui.Column(c).Grow(1).Gap(4).Children(func() {
		design.FieldLabel(c, p, label)
		ui.NumberInput(c, value, lo, hi, step)
	})
}

// sichuToggleChip is a selectable chip bound to one bool.
func (a *App) sichuToggleChip(c *ui.Context, p design.Palette, key any, label string, on *bool) {
	chip := design.Chip(c, p, key, label, *on)
	if chip.Clicked() {
		*on = !*on
	}
}

// sichuMapChip is a selectable chip bound to one entry of a set.
func (a *App) sichuMapChip(c *ui.Context, p design.Palette, key any, label, name string, set map[string]bool) {
	chip := design.Chip(c, p, key, label, set[name])
	if chip.Clicked() {
		set[name] = !set[name]
	}
}

// sichuOptionLabels returns option labels, optionally prefixed with 未填.
func sichuOptionLabels(options []sichu.Option, withUnchosen bool) []string {
	out := make([]string, 0, len(options)+1)
	if withUnchosen {
		out = append(out, unchosenLabel)
	}
	for _, o := range options {
		out = append(out, o.Label)
	}
	return out
}

// ---- Members page ------------------------------------------------------

func (a *App) sichuMembersPage(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Text(c, "同桌成员").FontSize(design.FontHeading).FontWeight(700).TextColor(p.Text).Role(ui.RoleHeading)
			if ui.PrimaryButton(c, "添加成员").Clicked() {
				a.openMemberEditor(domain.MemberProfile{})
			}
		})
		ui.Text(c, "本人卡从身体档案同步、只读；其他成员各自带忌口与过敏。配餐时按选中的成员取约束。").
			FontSize(12).TextColor(p.Muted).LineHeight(1.5).Margin(0, 0, design.Sp(c, 4), 0)

		a.section(c, p, "谁吃这一餐", func() {
			ui.Row(c).Gap(design.Sp(c, 2)).AlignItems(ui.Center).Wrap().Children(func() {
				a.sichuWhoControl(c, p)
				ui.Link(c, "回到私厨", "/sichu").FontSize(design.FontMeta)
			})
		})

		a.section(c, p, "成员", func() {
			if len(a.sichu.members) == 0 {
				ui.Text(c, "还没有成员卡。").FontSize(13).TextColor(p.Muted)
				return
			}
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				for i := range a.sichu.members {
					m := a.sichu.members[i]
					a.sichuMemberRow(c, p, m)
				}
			})
		})

		if a.sichu.memberNotice != "" {
			ui.Text(c, a.sichu.memberNotice).FontSize(12).TextColor(p.Accent).Margin(0, 8, 0, 0)
		}
	})

	if a.sichu.memberOpen {
		a.sichuMemberModal(c, p)
	}
	if a.sichu.confirmOpen {
		if ui.AlertDialog(c, &a.sichu.confirmOpen, "删除这位成员？", "成员卡删除后不能恢复。",
			"取消", "确认删除") == 1 {
			id := a.sichu.memberDelete
			a.deleteMember(id)
		}
	}
}

// sichuMemberRow draws one member card with its summary and actions.
func (a *App) sichuMemberRow(c *ui.Context, p design.Palette, m domain.MemberProfile) {
	row := ui.Row(c).FillWidth().Padding(design.Sp(c, 3.5), design.Sp(c, 3)).Gap(design.Sp(c, 3)).Radius(design.RadiusSmall).Background(p.Surface).
		Border(1, p.Border).AlignItems(ui.Center).Role(ui.RoleListItem)
	row.Children(func() {
		ui.Avatar(c, m.Name, nil).Size(34, 34).Background(p.AccentPale).TextColor(p.Accent)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			title := m.Name
			if m.IsSelf {
				title += "（本人）"
			}
			ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(p.Text).SingleLine()
			ui.Text(c, sichuMemberSummary(m)).FontSize(11).TextColor(p.Muted).SingleLine()
		})
		if m.IsSelf {
			ui.Text(c, "随身体档案同步").FontSize(11).TextColor(p.Faint)
			if ui.Button(c, "编辑档案").Clicked() {
				a.router.Push("/sichu/body")
			}
			return
		}
		if ui.Button(c, "编辑").Clicked() {
			a.openMemberEditor(m)
		}
		if design.Button(c, p, fmt.Sprintf("member-delete-%d", m.ID), "删除",
			design.ButtonDanger).Clicked() {
			a.askDeleteMember(m.ID)
		}
	})
}

// sichuMemberSummary writes one line about the member's body data and limits.
func sichuMemberSummary(m domain.MemberProfile) string {
	parts := make([]string, 0, 4)
	parts = append(parts, sichu.LabelOf(memberRoleOptions, m.Role))
	if m.Age > 0 {
		parts = append(parts, fmt.Sprintf("%d 岁", m.Age))
	}
	if m.HeightCm > 0 {
		parts = append(parts, fmt.Sprintf("%d cm", m.HeightCm))
	}
	if m.WeightKg > 0 {
		parts = append(parts, fmt.Sprintf("%d kg", m.WeightKg))
	}
	if tags := sichu.LabelsOf(sichu.HealthTagOptions,
		sichu.NormalizeNames(sichu.HealthTagOptions, m.HealthTags)); len(tags) > 0 {
		parts = append(parts, "忌："+strings.Join(tags, "、"))
	}
	if aller := sichu.LabelsOf(sichu.AllergenOptions,
		sichu.NormalizeNames(sichu.AllergenOptions, m.Allergens)); len(aller) > 0 {
		parts = append(parts, "过敏："+strings.Join(aller, "、"))
	}
	if goals := sichu.LabelsOf(sichu.GoalOptions,
		sichu.ParseJSONList(m.NutritionGoals)); len(goals) > 0 {
		parts = append(parts, "目标："+strings.Join(goals, "、"))
	}
	return strings.Join(parts, " · ")
}

// sichuMemberModal is the add/edit member form.
func (a *App) sichuMemberModal(c *ui.Context, p design.Palette) {
	d := &a.sichu.memberEdit
	design.ModalDialog(c, p, &a.sichu.memberOpen, 520, a.level, func() {
		ui.Text(c, "成员档案").FontSize(18).FontWeight(700).TextColor(p.Text)
		design.FieldLabel(c, p, "称呼")
		ui.TextInput(c, &d.name).Placeholder("例如：小满")
		ui.Row(c).Gap(16).Children(func() {
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				design.FieldLabel(c, p, "角色")
				ui.RadioGroup(c, func() {
					for _, o := range memberRoleOptions {
						ui.Radio(c, &d.role, o.Name, o.Label)
					}
				}).Row().Gap(10).Label("角色")
			})
			a.sichuNumberField(c, p, "年龄", &d.age, 0, 120, 1)
		})
		ui.Row(c).Gap(16).Children(func() {
			a.sichuNumberField(c, p, "身高 cm", &d.height, 0, 250, 1)
			a.sichuNumberField(c, p, "体重 kg", &d.weight, 0, 250, 1)
		})
		design.FieldLabel(c, p, "忌口与疾病")
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for i := range sichu.HealthTagOptions {
				o := sichu.HealthTagOptions[i]
				a.sichuMapChip(c, p, "m-health-"+o.Name, o.Label, o.Name, d.health)
			}
		})
		design.FieldLabel(c, p, "过敏源")
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for i := range sichu.AllergenOptions {
				o := sichu.AllergenOptions[i]
				a.sichuMapChip(c, p, "m-aller-"+o.Name, o.Label, o.Name, d.allergens)
			}
		})
		design.FieldLabel(c, p, "营养目标")
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for i := range sichu.GoalOptions {
				o := sichu.GoalOptions[i]
				a.sichuMapChip(c, p, "m-goal-"+o.Name, o.Label, o.Name, d.goals)
			}
		})
		if a.sichu.memberNotice != "" {
			ui.Text(c, a.sichu.memberNotice).FontSize(11).TextColor(p.Warning)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.sichu.memberOpen = false
			}
			if ui.PrimaryButton(c, "保存").Clicked() {
				a.saveMemberEdit()
			}
		})
	})
}

func sichuOptionLabelList(options []sichu.Option) []string {
	out := make([]string, 0, len(options))
	for _, o := range options {
		out = append(out, o.Label)
	}
	return out
}

// ---- AI settings page --------------------------------------------------

func (a *App) aiSettingsPage(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(0, 0, 18, 0).Children(func() {
			ui.BackButton(c, a.router)
			ui.Text(c, "AI 与配额").FontSize(12).TextColor(p.Muted)
		})

		a.section(c, p, "状态", func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 2.5)).Padding(design.Sp(c, 4)).Radius(design.RadiusCard).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					if a.ai.configured {
						design.Tag(c, p, "已配置", p.Success)
						ui.Text(c, "DeepSeek 已就绪").FontSize(13).TextColor(p.Text)
					} else {
						design.Tag(c, p, "未配置", p.Muted)
						ui.Text(c, "未配置密钥，配餐使用本地策略").FontSize(13).TextColor(p.Text)
					}
				})
				ui.Text(c, fmt.Sprintf("今日剩余 %d / %d 次（成功生成才计数）",
					a.ai.remaining, ai.DailyLimit)).FontSize(12).TextColor(p.Muted)
				ui.Text(c, "没有密钥、超配额或网络失败时，一律回退本地策略，页面不会卡住。").
					FontSize(11).TextColor(p.Faint).LineHeight(1.5)
			})
		})

		a.section(c, p, "密钥", func() {
			ui.Column(c).FillWidth().Gap(design.Sp(c, 2.5)).Padding(design.Sp(c, 4)).Radius(design.RadiusCard).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				design.FieldLabel(c, p, "DeepSeek API Key")
				in := ui.TextInput(c, &a.ai.keyInput).Placeholder("sk-…").Password().Label("DeepSeek API Key")
				if in.Submitted() {
					a.saveAIKey()
				}
				ui.Row(c).Gap(10).Children(func() {
					if ui.PrimaryButton(c, "保存密钥").Clicked() {
						a.saveAIKey()
					}
					if ui.Button(c, "清除密钥").Clicked() {
						a.clearAIKey()
					}
					if ui.Button(c, "测试连接").Clicked() {
						a.testAIConnection()
					}
					if a.ai.busy {
						loading.Arc(c, loading.Options{Size: 16, Color: p.Accent})
					}
				})
				if a.ai.notice != "" {
					ui.Text(c, a.ai.notice).FontSize(12).TextColor(p.Accent)
				}
				if a.ai.lastError != "" {
					ui.Text(c, a.ai.lastError).FontSize(11).TextColor(p.Warning)
				}
				ui.Text(c, "密钥只存在本机凭据管理器（Windows 凭据管理器），不进数据库、不进备份、不写日志。").
					FontSize(11).TextColor(p.Faint).LineHeight(1.5)
			})
		})

		a.section(c, p, "说明", func() {
			ui.Column(c).FillWidth().Gap(6).Padding(16).Radius(14).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				ui.Text(c, "AI 会拿到身体档案、忌口与可引用的库内菜式清单，返回的方案仍要过本地结构校验。").
					FontSize(12).TextColor(p.Muted).LineHeight(1.5)
				ui.Text(c, "校验不过会重试一次；仍不合规就整份回退本地策略，不静默删菜修补。").
					FontSize(12).TextColor(p.Muted).LineHeight(1.5)
				ui.Text(c, "热量与蛋白质都是估算，不构成医疗或营养结论。").FontSize(12).TextColor(p.Muted)
			})
		})
	})
}

// ---- shared labels -----------------------------------------------------

func sichuDescribe(plan sichu.MealPlan) string {
	return sichu.Describe(plan)
}

// smartSichuCounts is the "按人数智能配比" structure: one person gets a
// protein and a vegetable, two add the soup, and a bigger table adds another
// protein (then another vegetable) rather than more of everything.
func smartSichuCounts(servings int) sichu.Counts {
	switch {
	case servings <= 1:
		return sichu.Counts{Mains: 1, Vegs: 1}
	case servings <= 2:
		return sichu.Counts{Mains: 1, Vegs: 1, Soups: 1}
	case servings <= 4:
		return sichu.Counts{Mains: 2, Vegs: 1, Soups: 1}
	}
	return sichu.Counts{Mains: 2, Vegs: 2, Soups: 1}
}

func dishSlotLabel(slot sichu.Slot) string {
	return slot.Label()
}
