package sichu

import (
	"encoding/json"
	"strings"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// Option is one selectable value: the enum name written to the database and
// the Chinese label shown to the user.
type Option struct {
	Name  string
	Label string
}

// The option tables below are the writable vocabulary. Names match the
// Flutter enums so a backup written by the old app reads back unchanged.
var (
	SexOptions = []Option{
		{"male", "男"}, {"female", "女"},
	}
	ActivityOptions = []Option{
		{"sedentary", "久坐少动"}, {"light", "轻度活动"},
		{"moderate", "中度活动"}, {"active", "高强度活动"},
		{"veryActive", "极高强度"},
	}
	LifeStageOptions = []Option{
		{"", "无特殊阶段"}, {"preparingPregnancy", "备孕"},
		{"pregnancyEarly", "孕早期"}, {"pregnancyMid", "孕中期"},
		{"pregnancyLate", "孕晚期"}, {"lactation", "哺乳期"},
		{"menopause", "更年期"}, {"adolescence", "青少年发育期"},
		{"postSurgery", "术后恢复期"},
	}
	SpicyOptions = []Option{
		{"", "不限"}, {"none", "不辣"}, {"mild", "微辣"},
		{"medium", "中辣"}, {"hot", "重辣"},
	}
	SaltOptions = []Option{
		{"", "不限"}, {"light", "清淡"}, {"moderate", "适中"}, {"heavy", "重口"},
	}
	GoalOptions = []Option{
		{"balanced", "均衡"}, {"fatLoss", "减脂"}, {"muscleGain", "增肌"},
		{"sugarControl", "控糖"}, {"lowPurine", "低嘌呤"},
	}
	MealSlotOptions = []Option{
		{"breakfast", "早餐"}, {"lunch", "午餐"}, {"dinner", "晚餐"},
	}
	HealthTagOptions = []Option{
		{"diabetes", "糖尿病"}, {"hypertension", "高血压"}, {"gout", "痛风"},
		{"hyperlipidemia", "高血脂"}, {"gastro", "肠胃病"},
		{"kidneyLiver", "肾病/肝病"},
	}
	AllergenOptions = []Option{
		{"peanut", "花生"}, {"treeNut", "坚果"}, {"shellfish", "甲壳类"},
		{"fish", "鱼类"}, {"egg", "蛋类"}, {"milk", "奶类"},
		{"soy", "大豆"}, {"wheat", "小麦/麸质"},
		{"lactose", "乳糖不耐受"}, {"gluten", "麸质不耐受"},
	}
	RestrictionOptions = []Option{
		{"vegetarian", "吃斋"}, {"halal", "清真"}, {"allergy", "过敏"},
	}
)

// DefaultMealSlots is the private kitchen's default: lunch and dinner.
var DefaultMealSlots = []string{"lunch", "dinner"}

// LabelOf returns the Chinese label of an option name, or the name itself when
// it is not in the table.
func LabelOf(options []Option, name string) string {
	name = strings.TrimSpace(name)
	for _, o := range options {
		if o.Name == name {
			return o.Label
		}
	}
	return name
}

// NameOf resolves an option name from either the enum name or its Chinese
// label. Unknown values return the trimmed input, so hand-written backups with
// labels still survive.
func NameOf(options []Option, value string) string {
	value = strings.TrimSpace(value)
	for _, o := range options {
		if o.Name == value || (o.Label != "" && o.Label == value) {
			return o.Name
		}
	}
	return value
}

// LabelsOf maps a list of option names to their labels, skipping empties.
func LabelsOf(options []Option, names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if label := LabelOf(options, n); label != "" {
			out = append(out, label)
		}
	}
	return out
}

// SplitList parses a comma-separated enum list. The old app writes ASCII
// commas; hand-edited profiles may use Chinese punctuation, so both separate.
// Unknown blanks and duplicates are dropped.
func SplitList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '，', '、', ';', '；', ' ', '\t', '\n', '\u3000':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// JoinList writes a comma-separated enum list in the stored form.
func JoinList(names []string) string {
	return strings.Join(names, ",")
}

// NormalizeNames maps a stored list to option names, preserving order and
// dropping values this app does not know.
func NormalizeNames(options []Option, raw string) []string {
	names := SplitList(raw)
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		name := NameOf(options, n)
		if name == "" || seen[name] {
			continue
		}
		if !knownOption(options, name) {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// knownOption reports whether name is one of the table's names.
func knownOption(options []Option, name string) bool {
	for _, o := range options {
		if o.Name == name {
			return true
		}
	}
	return false
}

// ParseJSONList reads a JSON string array (nutrition goals are stored that
// way); a comma list also works so hand-written data is not silently lost.
func ParseJSONList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var out []string
		if err := json.Unmarshal([]byte(raw), &out); err == nil {
			return out
		}
	}
	return SplitList(raw)
}

// JoinJSONList writes a JSON string array in the stored form.
func JoinJSONList(names []string) string {
	if len(names) == 0 {
		return ""
	}
	b, err := json.Marshal(names)
	if err != nil {
		return ""
	}
	return string(b)
}

// NormalizeProfile fills the stored enum fields with their canonical names and
// drops values this app no longer knows. It keeps the profile writable even
// when it was imported from a hand-edited backup.
func NormalizeProfile(p domain.SichuProfile) domain.SichuProfile {
	p.Sex = NameOf(SexOptions, p.Sex)
	p.ActivityLevel = NameOf(ActivityOptions, p.ActivityLevel)
	p.LifeStage = NameOf(LifeStageOptions, p.LifeStage)
	p.NutritionGoal = NameOf(GoalOptions, p.NutritionGoal)
	p.HealthTags = JoinList(NormalizeNames(HealthTagOptions, p.HealthTags))
	p.Allergens = JoinList(NormalizeNames(AllergenOptions, p.Allergens))
	p.Restrictions = JoinList(NormalizeNames(RestrictionOptions, p.Restrictions))
	goals := make([]string, 0, 2)
	seenGoals := make(map[string]bool, 2)
	for _, g := range ParseJSONList(p.NutritionGoals) {
		name := NameOf(GoalOptions, g)
		if name == "" || seenGoals[name] || !knownOption(GoalOptions, name) {
			continue
		}
		seenGoals[name] = true
		goals = append(goals, name)
	}
	p.NutritionGoals = JoinJSONList(goals)
	meals := make([]string, 0, 3)
	for _, m := range NormalizeNames(MealSlotOptions, p.TargetMeals) {
		meals = append(meals, m)
	}
	p.TargetMeals = JoinList(meals)
	if p.ExploreRatio < 0 || p.ExploreRatio > 1 {
		p.ExploreRatio = 0.3
	}
	return p
}

// EffectiveRestrictions merges the private kitchen's own answers with the
// questionnaire: once the user has answered on the private kitchen page that
// answer wins, otherwise the questionnaire value is used.
func EffectiveRestrictions(p domain.SichuProfile, q domain.UserProfile) []string {
	raw := strings.TrimSpace(p.Restrictions)
	if raw == "" {
		raw = strings.TrimSpace(q.Restrictions)
	}
	return NormalizeNames(RestrictionOptions, raw)
}

// EffectiveAllergyNote is the free-text allergen list, same precedence as
// EffectiveRestrictions.
func EffectiveAllergyNote(p domain.SichuProfile, q domain.UserProfile) []string {
	raw := strings.TrimSpace(p.AllergyNote)
	if raw == "" {
		raw = strings.TrimSpace(q.AllergyNote)
	}
	return SplitList(raw)
}

// MealTitles returns the selected meal slots' labels in declaration order.
func MealTitles(names []string) []string {
	selected := make(map[string]bool, len(names))
	for _, n := range names {
		selected[NameOf(MealSlotOptions, n)] = true
	}
	out := make([]string, 0, 3)
	for _, o := range MealSlotOptions {
		if selected[o.Name] {
			out = append(out, o.Label)
		}
	}
	return out
}
