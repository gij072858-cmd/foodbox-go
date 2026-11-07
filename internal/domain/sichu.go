package domain

// SichuProfile is the self body profile: the single source of truth for the
// private kitchen's energy target. The member row with IsSelf mirrors it in
// one transaction, so the two never drift.
type SichuProfile struct {
	HeightCm       int
	WeightKg       int
	Age            int
	Sex            string
	ActivityLevel  string
	TargetWeightKg int
	BodyFatPct     float64
	LifeStage      string
	Allergens      string
	HealthStages   string
	HealthTags     string
	HealthNote     string
	NutritionGoal  string
	NutritionGoals string
	CalorieTarget  int
	Restrictions   string
	AllergyNote    string
	ExploreRatio   float64
	TargetMeals    string
}

// MemberProfile is one person at the table. The self member mirrors
// SichuProfile; other members carry their own body data and restrictions.
type MemberProfile struct {
	ID             int64
	Name           string
	Role           string
	IsSelf         bool
	HeightCm       int
	WeightKg       int
	Age            int
	Sex            string
	ActivityLevel  string
	TargetWeightKg int
	BodyFatPct     float64
	LifeStage      string
	Allergens      string
	HealthTags     string
	HealthStages   string
	HealthNote     string
	NutritionGoal  string
	NutritionGoals string
	CalorieTarget  int
}

// Nutrition goals the questionnaire offers. They are stored as the Chinese
// labels the Flutter app uses, so old data maps without translation.
const (
	GoalBalanced  = "均衡"
	GoalCutFat    = "减脂"
	GoalBulk      = "增肌"
	GoalSugar     = "控糖"
	GoalLowPurine = "低嘌呤"
)

// AllNutritionGoals lists the selectable goals in display order.
var AllNutritionGoals = []string{GoalBalanced, GoalCutFat, GoalBulk, GoalSugar, GoalLowPurine}

// Activity levels.
const (
	ActivitySedentary  = "sedentary"
	ActivityLight      = "light"
	ActivityModerate   = "moderate"
	ActivityActive     = "active"
	ActivityVeryActive = "veryActive"
)

// ActivityLabel is the Chinese label for an activity level.
func ActivityLabel(level string) string {
	switch level {
	case ActivityLight:
		return "轻度活动"
	case ActivityModerate:
		return "中度活动"
	case ActivityActive:
		return "高强度活动"
	case ActivityVeryActive:
		return "极高强度"
	default:
		return "久坐少动"
	}
}
