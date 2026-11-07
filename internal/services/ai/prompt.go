package ai

import (
	"fmt"
	"strings"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/sichu"
)

// promptCandidateLimit is how many library dishes the prompt lists. All 900+
// would cost a lot of tokens for little extra choice; the planner's ordering
// already puts the user's likely dishes first.
const promptCandidateLimit = 240

// BuildUserPrompt turns the planner's input into the one user message the
// model sees: the diner's profile, the hard limits, the structure and the
// exact dishes it may cite.
//
// It is exported so the settings page's "test connection" can reuse the shape
// if it needs to, and so the prompt stays testable without HTTP.
func BuildUserPrompt(in sichu.PlanInput) string {
	prefs := in.Preferences()
	var b strings.Builder

	fmt.Fprintf(&b, "【餐型】%s\n", in.Style.Label())
	switch in.Style {
	case sichu.StyleOneBowl:
		b.WriteString("每餐 1 道自足的一碗端核心，最多再加 1 道汤羹；不要拼第二份主食。\n")
	case sichu.StyleCustom:
		fmt.Fprintf(&b, "每餐恰好 %d 道：%d 道荤菜（蛋白主料）+ %d 道蔬菜类菜 + %d 道汤羹（做法为汤羹）。\n",
			in.Counts.Total(), in.Counts.Mains, in.Counts.Vegs, in.Counts.Soups)
	default:
		b.WriteString("每餐恰好 3 道：1 道荤菜（蛋白主料）+ 1 道蔬菜类菜 + 1 道汤羹（做法为汤羹）。\n")
	}
	fmt.Fprintf(&b, "【餐段】%s（共 %d 餐，每餐一组）\n",
		strings.Join(mealTitles(in), "、"), len(mealTitles(in)))
	fmt.Fprintf(&b, "【份数】%d 人\n", in.Servings)

	ratio := int(in.ExploreRatioOrDefault()*100 + 0.5)
	fmt.Fprintf(&b, "【稳妥与探索】探索约 %d%%（按比例安排没做过的菜）\n", ratio)

	b.WriteString("【身体档案】")
	parts := make([]string, 0, 6)
	if in.Profile.HeightCm > 0 {
		parts = append(parts, fmt.Sprintf("身高 %d cm", in.Profile.HeightCm))
	}
	if in.Profile.WeightKg > 0 {
		parts = append(parts, fmt.Sprintf("体重 %d kg", in.Profile.WeightKg))
	}
	if in.Profile.Age > 0 {
		parts = append(parts, fmt.Sprintf("年龄 %d", in.Profile.Age))
	}
	if in.Profile.Sex != "" {
		parts = append(parts, "性别 "+sichu.LabelOf(sichu.SexOptions, in.Profile.Sex))
	}
	if in.Profile.ActivityLevel != "" {
		parts = append(parts, "活动量 "+sichu.LabelOf(sichu.ActivityOptions, in.Profile.ActivityLevel))
	}
	if in.Profile.LifeStage != "" {
		parts = append(parts, "生理阶段 "+sichu.LabelOf(sichu.LifeStageOptions, in.Profile.LifeStage))
	}
	if len(parts) == 0 {
		parts = append(parts, "未填写")
	}
	b.WriteString(strings.Join(parts, "，") + "\n")

	if summary := prefs.Summary(); len(summary) > 0 {
		fmt.Fprintf(&b, "【忌口、疾病与口味】%s\n", strings.Join(summary, "；"))
	} else {
		b.WriteString("【忌口、疾病与口味】无特别限制\n")
	}
	if in.MaxMinutes > 0 {
		fmt.Fprintf(&b, "【单道时长上限】%d 分钟\n", in.MaxMinutes)
	}

	b.WriteString("【可引用的库内菜式】每行：id | 菜名 | 主料 | 做法 | 时长 | 主要食材\n")
	candidates := sichu.LegalRecipes(in)
	if len(candidates) > promptCandidateLimit {
		candidates = candidates[:promptCandidateLimit]
	}
	for _, rec := range candidates {
		fmt.Fprintf(&b, "%d | %s | %s | %s | %s | %s\n",
			rec.ID, rec.Name, rec.MainCategory, rec.Cuisine,
			durationText(rec), strings.Join(requiredIngredients(rec, 5), "、"))
	}
	if len(candidates) == 0 {
		b.WriteString("（本次没有可引用的库内菜式）\n")
	}

	b.WriteString("\n要求：只输出一个 JSON 对象 {\"meals\":[{\"title\":\"午餐\",\"mealStyle\":\"" +
		string(in.Style) + "\"," +
		"\"dishes\":[{\"name\":\"菜名\",\"recipeId\":12,\"brief\":\"一句话原因\"," +
		"\"durationMinutes\":20,\"waitMinutes\":0,\"kcal\":320,\"proteinG\":28}]}],\"note\":\"一句取舍\"}。\n")
	b.WriteString("recipeId 只能引用上面清单里的数字；本次不要给 draft，不要输出解释文字。\n")
	return b.String()
}

func mealTitles(in sichu.PlanInput) []string {
	if len(in.Meals) > 0 {
		return in.Meals
	}
	return sichu.MealTitles(sichu.DefaultMealSlots)
}

func durationText(rec domain.Recipe) string {
	if text := rec.DurationText(); text != "" {
		return text
	}
	return "时长未填"
}

func requiredIngredients(rec domain.Recipe, n int) []string {
	out := make([]string, 0, n)
	for _, ing := range rec.Ingredients {
		if !ing.Required {
			continue
		}
		if name := strings.TrimSpace(ing.Name); name != "" {
			out = append(out, name)
		}
		if len(out) >= n {
			break
		}
	}
	return out
}
