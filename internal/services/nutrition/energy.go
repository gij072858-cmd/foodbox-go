package nutrition

// The energy engine: Mifflin-St Jeor for the basal rate, an activity factor
// for the daily burn, the goal's adjustment for the target, and then macros.
//
// It is pure arithmetic, no table lookups, so it can be tested against hand
// computed fixtures. When the body profile is too incomplete to compute a
// trustworthy number it returns zeroes and a Note naming the missing inputs:
// the dashboard shows the gap instead of inventing a target.

import (
	"fmt"
	"math"
	"strings"
)

// minTargetKcal is the floor under a computed daily target. A deficit plan
// must never produce a number a home cook might mistake for a safe intake.
const minTargetKcal = 1200

// activityFactors maps the profile's activity level to the TDEE multiplier.
var activityFactors = []struct {
	keys   []string
	factor float64
	label  string
}{
	{[]string{"sedentary", "久坐", "少动", "轻体力"}, 1.2, "久坐少动"},
	{[]string{"veryactive", "极高", "高强度训练"}, 1.9, "极高强度"},
	{[]string{"light", "轻度"}, 1.375, "轻度活动"},
	{[]string{"moderate", "中度"}, 1.55, "中度活动"},
	{[]string{"active", "高强度"}, 1.725, "高强度活动"},
}

// goalPlan is what a nutrition goal changes: the calorie adjustment, the
// protein allowance per kg of body weight, and fat's share of the target.
type goalPlan struct {
	label        string
	delta        float64
	proteinPerKg float64
	fatShare     float64
}

// goalPlans are checked in order, so 减脂 wins over 增肌 if both are selected.
var goalPlans = []struct {
	keys []string
	plan goalPlan
}{
	{[]string{"减脂", "fatloss", "fat", "cut"}, goalPlan{label: "减脂", delta: -0.15, proteinPerKg: 1.6, fatShare: 0.25}},
	{[]string{"增肌", "muscle", "bulk"}, goalPlan{label: "增肌", delta: 0.10, proteinPerKg: 1.8, fatShare: 0.30}},
	{[]string{"控糖", "sugar", "低糖"}, goalPlan{label: "控糖", delta: 0, proteinPerKg: 1.2, fatShare: 0.275}},
	{[]string{"低嘌呤", "purine", "痛风"}, goalPlan{label: "低嘌呤", delta: 0, proteinPerKg: 1.2, fatShare: 0.275}},
}

// balancedPlan is the fallback when no goal was chosen.
var balancedPlan = goalPlan{label: "均衡", delta: 0, proteinPerKg: 1.2, fatShare: 0.275}

// computeEnergy is EnergyFor's implementation.
func computeEnergy(in EnergyInput) Energy {
	var missing []string
	if in.HeightCm <= 0 {
		missing = append(missing, "身高")
	}
	if in.WeightKg <= 0 {
		missing = append(missing, "体重")
	}
	if in.Age <= 0 {
		missing = append(missing, "年龄")
	}
	if len(missing) > 0 {
		return Energy{Note: "还缺" + strings.Join(missing, "、") + "，补齐后再算热量目标"}
	}

	sexAdjust, sexKnown := sexAdjustment(in.Sex)
	factor, activityKnown := activityFactor(in.ActivityLevel)
	stageKcal, stageLabel := lifeStageAdjustment(in.LifeStage)
	plan := goalPlanFor(in.Goals)

	rawBMR := 10*float64(in.WeightKg) + 6.25*float64(in.HeightCm) - 5*float64(in.Age) + sexAdjust
	rawTDEE := rawBMR*factor + stageKcal
	rawTarget := rawTDEE * (1 + plan.delta)
	// A deficit can dip under the floor; the floor wins and the note says so.
	if rawTarget < minTargetKcal {
		rawTarget = minTargetKcal
	}

	energy := Energy{
		BMR:        int(math.Round(rawBMR)),
		TDEE:       int(math.Round(rawTDEE)),
		TargetKcal: int(math.Round(rawTarget)),
	}
	energy.ProteinG, energy.FatG, energy.CarbG = macroTargets(in.WeightKg, energy.TargetKcal, plan)

	var notes []string
	if !sexKnown {
		notes = append(notes, "性别未填，按男女均值估算")
	}
	if !activityKnown {
		notes = append(notes, "活动量未填，按久坐估算")
	}
	if stageLabel != "" {
		notes = append(notes, fmt.Sprintf("%s，热量 %+d kcal", stageLabel, int(stageKcal)))
	}
	energy.Note = strings.Join(notes, "；")
	return energy
}

// ApplyTarget re-derives the macros for a calorie target the owner set by
// hand. It keeps the basal numbers and leaves the note saying where the
// target came from.
func ApplyTarget(e Energy, in EnergyInput, targetKcal int) Energy {
	if targetKcal <= 0 || in.WeightKg <= 0 || e.BMR <= 0 {
		return e
	}
	plan := goalPlanFor(in.Goals)
	e.TargetKcal = targetKcal
	e.ProteinG, e.FatG, e.CarbG = macroTargets(in.WeightKg, targetKcal, plan)
	note := "目标热量来自身体档案的自定值"
	if e.Note != "" {
		note = e.Note + "；" + note
	}
	e.Note = note
	return e
}

// macroTargets splits a target into grams. Protein follows the goal's
// allowance per kg of body weight, fat takes its share of the target, and
// carbohydrate fills the remainder.
func macroTargets(weightKg, targetKcal int, plan goalPlan) (proteinG, fatG, carbG int) {
	proteinG = int(math.Round(float64(weightKg) * plan.proteinPerKg))
	fatG = int(math.Round(float64(targetKcal) * plan.fatShare / 9))
	if remaining := targetKcal - proteinG*4 - fatG*9; remaining > 0 {
		carbG = int(math.Round(float64(remaining) / 4))
	}
	return proteinG, fatG, carbG
}

// sexAdjustment is the tail of the Mifflin-St Jeor formula. An unknown sex
// takes the midpoint of +5 and -161 rather than pretending to be one of them.
func sexAdjustment(sex string) (float64, bool) {
	s := strings.ToLower(strings.TrimSpace(sex))
	switch {
	case s == "male", s == "m", strings.Contains(s, "男"):
		return 5, true
	case s == "female", s == "f", strings.Contains(s, "女"):
		return -161, true
	}
	return (5 + -161) / 2.0, false
}

// activityFactor returns the TDEE multiplier, defaulting to sedentary when the
// profile has none.
func activityFactor(level string) (float64, bool) {
	if strings.TrimSpace(level) != "" {
		for _, row := range activityFactors {
			if containsAny(strings.ToLower(level), row.keys...) {
				return row.factor, true
			}
		}
	}
	return 1.2, false
}

// lifeStageAdjustment is the extra daily energy a life stage needs, in kcal,
// plus the Chinese phrase the note uses.
func lifeStageAdjustment(stage string) (float64, string) {
	s := strings.ToLower(strings.TrimSpace(stage))
	switch {
	case s == "", s == "none":
		return 0, ""
	case containsAny(s, "preparing", "备孕"):
		return 0, "备孕期无热量追加"
	case containsAny(s, "lactat", "哺乳", "月子"):
		return 500, "哺乳期"
	case containsAny(s, "pregnan", "孕"):
		return 300, "孕期"
	case containsAny(s, "postsurgery", "postpartum", "术后", "恢复"):
		return 200, "恢复期"
	case containsAny(s, "menopaus", "更年"):
		return 0, ""
	}
	return 0, ""
}

// goalPlanFor picks the plan for the selected goals, most restrictive first.
func goalPlanFor(goals []string) goalPlan {
	for _, row := range goalPlans {
		for _, goal := range goals {
			if containsAny(strings.ToLower(goal), row.keys...) {
				return row.plan
			}
		}
	}
	return balancedPlan
}
