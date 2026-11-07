package sichu

import (
	"encoding/json"
	"strings"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// Preferences is the hard and soft limit set the planner works with: hard
// limits filter the candidate pool, soft ones only reorder it.
type Preferences struct {
	// Halal and Vegetarian come from the questionnaire or the private
	// kitchen's own answer.
	Halal      bool
	Vegetarian bool

	// HealthTags are chronic-condition enum names (diabetes, gout, ...).
	HealthTags []string
	// HealthStages narrows a condition (e.g. goutAcute) with extra blocks.
	HealthStages map[string]string

	// Allergens are structured allergen category names; AllergyNote is the
	// free-text list the user typed.
	Allergens   []string
	AllergyNote []string

	// Spicy and Salt are questionnaire taste preferences (enum names).
	Spicy string
	Salt  string

	// Goals are nutrition goal enum names.
	Goals []string
}

// PreferencesFrom folds the body profile, the questionnaire and every diner at
// the table into one limit set. Where a diner declares something, that limit
// applies to the whole meal: one dish has to suit everyone.
func PreferencesFrom(p domain.SichuProfile, q domain.UserProfile, diners []domain.MemberProfile) Preferences {
	prefs := Preferences{
		HealthTags:   NormalizeNames(HealthTagOptions, p.HealthTags),
		HealthStages: parseHealthStages(p.HealthStages),
		Allergens:    NormalizeNames(AllergenOptions, p.Allergens),
		AllergyNote:  EffectiveAllergyNote(p, q),
		Spicy:        NameOf(SpicyOptions, q.Spicy),
		Salt:         NameOf(SaltOptions, q.Saltiness),
	}
	for _, r := range EffectiveRestrictions(p, q) {
		switch r {
		case "halal":
			prefs.Halal = true
		case "vegetarian":
			prefs.Vegetarian = true
		}
	}
	tags := make(map[string]bool, len(prefs.HealthTags))
	for _, t := range prefs.HealthTags {
		tags[t] = true
	}
	allergens := make(map[string]bool, len(prefs.Allergens))
	for _, a := range prefs.Allergens {
		allergens[a] = true
	}
	for _, m := range diners {
		for _, t := range NormalizeNames(HealthTagOptions, m.HealthTags) {
			tags[t] = true
		}
		for _, a := range NormalizeNames(AllergenOptions, m.Allergens) {
			allergens[a] = true
		}
		for tag, stage := range parseHealthStages(m.HealthStages) {
			if prefs.HealthStages == nil {
				prefs.HealthStages = make(map[string]string)
			}
			if prefs.HealthStages[tag] == "" {
				prefs.HealthStages[tag] = stage
			}
		}
	}
	prefs.HealthTags = prefs.HealthTags[:0]
	for _, o := range HealthTagOptions {
		if tags[o.Name] {
			prefs.HealthTags = append(prefs.HealthTags, o.Name)
		}
	}
	prefs.Allergens = prefs.Allergens[:0]
	for _, o := range AllergenOptions {
		if allergens[o.Name] {
			prefs.Allergens = append(prefs.Allergens, o.Name)
		}
	}
	if goals := ParseJSONList(p.NutritionGoals); len(goals) > 0 {
		for _, g := range goals {
			if name := NameOf(GoalOptions, g); name != "" {
				prefs.Goals = append(prefs.Goals, name)
			}
		}
	} else if g := NameOf(GoalOptions, p.NutritionGoal); g != "" {
		prefs.Goals = []string{g}
	}
	return prefs
}

// hasGoal reports whether a nutrition goal is selected.
func (p Preferences) hasGoal(name string) bool {
	for _, g := range p.Goals {
		if g == name {
			return true
		}
	}
	return false
}

// Hard reports whether any hard limit is set.
func (p Preferences) Hard() bool {
	return p.Halal || p.Vegetarian || len(p.HealthTags) > 0 ||
		len(p.Allergens) > 0 || len(p.AllergyNote) > 0
}

// DishViolation checks one dish against every hard limit. It returns "" when
// the dish is safe, otherwise a short Chinese reason naming the conflict.
//
// ingredientNames must include seasonings: an allergen inside a sauce still
// matters. The dish name is checked too, because some limits are about the
// cooking method rather than an ingredient (糖醋, 拔丝, 油炸).
func (p Preferences) DishViolation(name string, ingredientNames []string) string {
	names := make([]string, 0, len(ingredientNames)+1)
	if n := strings.TrimSpace(name); n != "" {
		names = append(names, n)
	}
	names = append(names, ingredientNames...)
	if len(names) == 0 {
		return ""
	}

	if p.Halal {
		if hits := matched(names, porkKeywords, alcoholKeywords); len(hits) > 0 {
			return "与「清真」冲突：含 " + strings.Join(hits, "、")
		}
	}
	if p.Vegetarian {
		if hits := matched(names, meatKeywords); len(hits) > 0 {
			return "与「吃斋」冲突：含 " + strings.Join(hits, "、")
		}
	}
	if len(p.Allergens) > 0 {
		var categoryLabels []string
		for _, a := range p.Allergens {
			keywords := allergenKeywords[a]
			if len(keywords) == 0 {
				continue
			}
			if len(matched(names, keywords)) > 0 {
				categoryLabels = append(categoryLabels, LabelOf(AllergenOptions, a))
			}
		}
		if len(categoryLabels) > 0 {
			return "与「过敏」冲突：含 " + strings.Join(categoryLabels, "、") + "类食材"
		}
	}
	if len(p.AllergyNote) > 0 {
		if hits := matched(names, p.AllergyNote); len(hits) > 0 {
			return "与「过敏」冲突：含 " + strings.Join(hits, "、")
		}
	}
	for _, tag := range p.HealthTags {
		keywords := append([]string{}, healthKeywords[tag]...)
		if stage := p.HealthStages[tag]; stage != "" {
			keywords = append(keywords, healthStageKeywords[stage]...)
		}
		if len(keywords) == 0 {
			continue
		}
		if hits := matched(names, keywords); len(hits) > 0 {
			label := LabelOf(HealthTagOptions, tag)
			if stage := p.HealthStages[tag]; stage != "" {
				label += "·" + LabelOf(HealthStageOptions, stage)
			}
			return "与「" + label + "」冲突：含 " + strings.Join(hits, "、")
		}
	}
	return ""
}

// Summary lists the active limits in one short line each, for the entry page.
func (p Preferences) Summary() []string {
	out := make([]string, 0, 8)
	if p.Halal {
		out = append(out, "清真")
	}
	if p.Vegetarian {
		out = append(out, "吃斋")
	}
	for _, t := range p.HealthTags {
		label := LabelOf(HealthTagOptions, t)
		if stage := p.HealthStages[t]; stage != "" {
			label += "·" + LabelOf(HealthStageOptions, stage)
		}
		out = append(out, label)
	}
	if len(p.Allergens) > 0 {
		out = append(out, "过敏："+strings.Join(LabelsOf(AllergenOptions, p.Allergens), "、"))
	}
	if len(p.AllergyNote) > 0 {
		out = append(out, "过敏："+strings.Join(p.AllergyNote, "、"))
	}
	if p.Spicy != "" && p.Spicy != "none" {
		out = append(out, "口味 "+LabelOf(SpicyOptions, p.Spicy))
	}
	if p.Salt == "light" {
		out = append(out, "口味 清淡")
	}
	if len(p.Goals) > 0 {
		out = append(out, "目标 "+strings.Join(LabelsOf(GoalOptions, p.Goals), "、"))
	}
	return out
}

// HealthStageOptions are the condition stages the old app could store. The
// private kitchen page does not ask for them yet, but profiles imported from
// the old app may carry them, so the planner honours them.
var HealthStageOptions = []Option{
	{"unset", "未分期"}, {"goutAcute", "急性发作期"},
	{"goutRemission", "缓解期"}, {"dmPoor", "血糖控制差"},
	{"dmFair", "控制一般"}, {"dmGood", "控制良好"},
	{"htnMild", "轻度"}, {"htnSevere", "重度"},
	{"giUlcer", "胃溃疡"}, {"giReflux", "反流"}, {"giIbs", "肠易激"},
	{"kidneyDisease", "慢性肾病"}, {"liverDisease", "肝病"},
	{"lipidSevere", "重度"},
}

func parseHealthStages(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		tag := NameOf(HealthTagOptions, k)
		if tag == "" || !validHealthTag(tag) {
			continue
		}
		stage := NameOf(HealthStageOptions, v)
		if stage == "" || stage == "unset" {
			continue
		}
		out[tag] = stage
	}
	return out
}

func validHealthTag(name string) bool {
	for _, o := range HealthTagOptions {
		if o.Name == name {
			return true
		}
	}
	return false
}

// matched returns the keywords found in any of the names, in table order.
func matched(names []string, keywordLists ...[]string) []string {
	var hits []string
	for _, keywords := range keywordLists {
		for _, kw := range keywords {
			found := false
			for _, name := range names {
				if strings.Contains(strings.ToLower(name), strings.ToLower(kw)) {
					found = true
					break
				}
			}
			if found && !containsString(hits, kw) {
				hits = append(hits, kw)
			}
		}
	}
	return hits
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// porkKeywords is the halal pork family.
var porkKeywords = []string{
	"猪肉", "猪", "五花", "里脊", "培根", "火腿", "腊肠", "排骨", "猪蹄",
	"猪油", "梅花肉",
}

// alcoholKeywords is alcohol cooked into a dish. Cooking wine is deliberately
// not blocked: it is a seasoning in home cooking, and blocking it would rule
// out most meat dishes.
var alcoholKeywords = []string{"啤酒", "米酒", "酒糟", "醉", "朗姆", "红酒炖"}

// meatKeywords is the vegetarian block: meat, poultry, seafood and eggs.
var meatKeywords = []string{
	"肉", "鸡", "鸭", "鹅", "鸽", "鱼", "虾", "蟹", "贝", "蛤", "鲍", "螺",
	"蚝", "蛋", "排骨", "培根", "火腿", "香肠",
}

// allergenKeywords maps a structured allergen category to ingredient words.
// It is a safety net, not a nutrition database: false positives cost one
// recipe, false negatives cost a diner's health.
var allergenKeywords = map[string][]string{
	"peanut": {"花生"},
	"treeNut": {
		"核桃", "腰果", "杏仁", "榛子", "开心果", "夏威夷果", "松子",
		"巴旦木", "碧根果", "坚果",
	},
	"shellfish": {
		"虾", "蟹", "龙虾", "贝", "蛤", "蚬", "扇贝", "生蚝", "蚝", "鲍",
		"花甲", "蛏子", "淡菜", "皮皮虾",
	},
	"fish": {
		"鱼", "鲈", "鲫", "鲤", "鲑", "三文鱼", "鳕", "带鱼", "黄鱼",
		"龙利", "鳗", "鳝", "鲳", "鲅", "金枪", "沙丁", "鱿鱼", "章鱼",
		"墨鱼", "鳜", "鲶", "罗非",
	},
	"egg": {"蛋"},
	"milk": {
		"牛奶", "奶", "黄油", "奶油", "芝士", "奶酪", "炼乳", "酸奶",
		"乳酪", "淡奶油",
	},
	"soy": {
		"大豆", "黄豆", "豆腐", "豆浆", "豆干", "腐竹", "豆皮", "酱油",
		"豆瓣", "豆豉", "毛豆", "豆芽",
	},
	"wheat": {
		"小麦", "面粉", "面条", "馒头", "面包", "饺皮", "馄饨皮", "意面",
		"拉面", "面筋", "蛋糕", "饼干", "方便面", "乌冬", "挂面", "面",
	},
	"lactose": {
		"牛奶", "奶", "酸奶", "炼乳", "乳酪", "芝士", "奶酪", "淡奶油",
	},
	"gluten": {
		"小麦", "面粉", "面条", "馒头", "面包", "饺皮", "馄饨皮", "意面",
		"拉面", "面筋", "蛋糕", "饼干", "方便面", "乌冬", "挂面", "面",
	},
}

// healthKeywords maps a condition to blocked ingredient or method words.
// Paths run over the dish name too, so processed methods are caught even when
// the ingredient list is incomplete.
var healthKeywords = map[string][]string{
	"diabetes": {
		"白米", "糯米", "年糕", "糖", "蜂蜜", "土豆", "红薯", "紫薯", "南瓜",
		"粉条", "粉丝", "粥", "拔丝", "甜羹", "糖醋",
	},
	"hypertension": {
		"咸菜", "酱菜", "腌", "腊肉", "香肠", "火腿", "豆腐乳", "榨菜",
	},
	"gout": {
		"内脏", "腰子", "肝", "脑花", "老火汤", "高汤", "浓汤", "啤酒",
		"花甲", "蛤蜊", "生蚝",
	},
	"hyperlipidemia": {
		"油炸", "酥", "肥肉", "五花肉", "猪油", "黄油", "奶油", "炸",
	},
	"gastro": {"辣", "花椒", "胡椒", "酒", "油炸", "腌", "酸辣"},
	"kidneyLiver": {
		"内脏", "腰子", "肝", "老火汤", "高汤", "浓汤", "腊肉", "香肠",
		"火腿", "咸菜", "腐乳",
	},
}

// healthStageKeywords adds a stricter block for a condition's stage.
var healthStageKeywords = map[string][]string{
	"goutAcute": {
		"牛", "羊", "猪", "鸭", "内脏", "沙丁", "凤尾", "鲭", "贝", "虾",
		"蟹", "蛤", "生蚝",
	},
	"dmPoor":        {"糖", "蜜", "甜", "拔丝", "粥", "糕"},
	"htnSevere":     {"酱油", "生抽", "老抽", "蚝油", "味精", "鸡精", "豆瓣", "豆豉", "腐乳", "咸", "酱"},
	"giUlcer":       {"辣", "酸", "油炸", "硬", "粗粮"},
	"giReflux":      {"辣", "酸", "油炸", "咖啡", "巧克力", "薄荷"},
	"giIbs":         {"洋葱", "蒜", "韭菜", "豆", "奶", "麦"},
	"kidneyDisease": {"内脏", "浓汤", "高汤", "腌", "咸"},
	"liverDisease":  {"酒", "油炸", "肥肉", "霉"},
	"lipidSevere":   {"油炸", "肥肉", "猪油", "黄油", "奶油"},
	"goutRemission": {},
	"dmFair":        {"糖", "蜜", "拔丝"},
	"dmGood":        {"糖", "蜜"},
	"htnMild":       {"咸菜", "酱菜", "腌"},
}
