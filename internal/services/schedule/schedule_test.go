package schedule

import (
	"reflect"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
)

func minutes(v float64) *float64 { return &v }

// goldenRecipes is the three-dish sample the timeline test pins: two board
// steps, three stove steps, three hands-on steps and one passive wait.
func goldenRecipes() []domain.Recipe {
	return []domain.Recipe{
		{ID: 1, Name: "青椒肉丝", BaseServings: 2},
		{ID: 2, Name: "番茄蛋汤", BaseServings: 2},
		{ID: 3, Name: "清炖牛腩", BaseServings: 3},
	}
}

func goldenSteps() map[int64][]domain.Step {
	return map[int64][]domain.Step{
		1: {
			{OrderIndex: 1, Instruction: "青椒切丝", DurationMinutes: minutes(2), Resources: []string{"board"}},
			{OrderIndex: 2, Instruction: "滑炒肉丝", DurationMinutes: minutes(5), Resources: []string{"stove"}},
			{OrderIndex: 3, Instruction: "装盘", DurationMinutes: minutes(1), Resources: []string{"hand"}},
		},
		2: {
			{OrderIndex: 1, Instruction: "打蛋", DurationMinutes: minutes(2), Resources: []string{"hand"}},
			{OrderIndex: 2, Instruction: "煮汤", DurationMinutes: minutes(3), Resources: []string{"stove"}},
		},
		3: {
			{OrderIndex: 1, Instruction: "牛腩切块", DurationMinutes: minutes(3), Resources: []string{"board"}},
			// A passive braise: it does not hold a burner in the plan.
			{OrderIndex: 2, Instruction: "小火炖", DurationMinutes: minutes(10), Resources: []string{"waiting"}},
			{OrderIndex: 3, Instruction: "调味", DurationMinutes: minutes(1), Resources: nil},
		},
	}
}

// TestBuildTimelineGolden pins the whole plan for the three-dish sample: the
// totals, the lane order and every block's minutes. It is the M4 acceptance
// fixture — if the planner changes, this test has to be re-derived on purpose.
func TestBuildTimelineGolden(t *testing.T) {
	got := BuildTimeline(goldenRecipes(), goldenSteps(), map[int64]int{3: 4}, 2)

	if got.TotalMinutes != 14 || got.SerialMinutes != 27 || got.SavedMinutes != 13 {
		t.Fatalf("totals = total %d serial %d saved %d, want 14/27/13",
			got.TotalMinutes, got.SerialMinutes, got.SavedMinutes)
	}
	if got.StoveCount != 2 {
		t.Errorf("stove count = %d", got.StoveCount)
	}
	wantLanes := []struct {
		label string
		start int
		end   int
		steps []string
	}{
		{"灶台 1", 2, 7, []string{"青椒肉丝#2@2-7"}},
		{"灶台 2", 2, 5, []string{"番茄蛋汤#2@2-5"}},
		{"案板 1", 0, 2, []string{"青椒肉丝#1@0-2"}},
		{"案板 2", 0, 3, []string{"清炖牛腩#1@0-3"}},
		{"动手 1", 0, 14, []string{"番茄蛋汤#1@0-2", "青椒肉丝#3@7-8", "清炖牛腩#3@13-14"}},
		{"等待 1", 3, 13, []string{"清炖牛腩#2@3-13"}},
	}
	if len(got.Lanes) != len(wantLanes) {
		for _, lane := range got.Lanes {
			t.Logf("lane %s %d-%d %v", lane.Label, lane.Start, lane.End, laneSteps(lane))
		}
		t.Fatalf("lanes = %d, want %d", len(got.Lanes), len(wantLanes))
	}
	for i, want := range wantLanes {
		lane := got.Lanes[i]
		if lane.Label != want.label || lane.Start != want.start || lane.End != want.end {
			t.Errorf("lane %d = %s %d-%d, want %s %d-%d",
				i, lane.Label, lane.Start, lane.End, want.label, want.start, want.end)
		}
		if steps := laneSteps(lane); !reflect.DeepEqual(steps, want.steps) {
			t.Errorf("lane %s steps = %v, want %v", lane.Label, steps, want.steps)
		}
	}
}

func laneSteps(lane domain.TimelineLane) []string {
	out := make([]string, 0, len(lane.Steps))
	for _, st := range lane.Steps {
		out = append(out, st.RecipeName+"#"+itoa(st.StepIndex)+"@"+itoa(st.Start)+"-"+itoa(st.End))
	}
	return out
}

// TestBuildTimelineDeterministic runs the same input twice: the plan is a
// pure function of the queue, so the two results must match exactly.
func TestBuildTimelineDeterministic(t *testing.T) {
	first := BuildTimeline(goldenRecipes(), goldenSteps(), nil, 2)
	second := BuildTimeline(goldenRecipes(), goldenSteps(), nil, 2)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("the same queue produced two different timelines")
	}
}

// TestBuildTimelineStoveCapacity pins the capacity rule: three five-minute
// stove steps fit side by side only when the kitchen has three burners.
func TestBuildTimelineStoveCapacity(t *testing.T) {
	recipes := []domain.Recipe{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}
	steps := map[int64][]domain.Step{
		1: {{OrderIndex: 1, DurationMinutes: minutes(5), Resources: []string{"stove"}}},
		2: {{OrderIndex: 1, DurationMinutes: minutes(5), Resources: []string{"stove"}}},
		3: {{OrderIndex: 1, DurationMinutes: minutes(5), Resources: []string{"stove"}}},
	}
	two := BuildTimeline(recipes, steps, nil, 2)
	if two.TotalMinutes != 10 {
		t.Errorf("two burners: total = %d, want 10", two.TotalMinutes)
	}
	three := BuildTimeline(recipes, steps, nil, 3)
	if three.TotalMinutes != 5 {
		t.Errorf("three burners: total = %d, want 5", three.TotalMinutes)
	}
	// A missing or absurd stove count falls back to the default, not to zero.
	clamped := BuildTimeline(recipes, steps, nil, 9)
	if clamped.StoveCount != MaxStoveCount {
		t.Errorf("stove count = %d, want %d", clamped.StoveCount, MaxStoveCount)
	}
	if fallback := BuildTimeline(recipes, steps, nil, 0); fallback.StoveCount != DefaultStoveCount {
		t.Errorf("default stove count = %d", fallback.StoveCount)
	}
}

// TestBuildTimelineKeepsStepOrder checks the chain rule: no step of a dish
// starts before the dish's previous step ends.
func TestBuildTimelineKeepsStepOrder(t *testing.T) {
	tl := BuildTimeline(goldenRecipes(), goldenSteps(), nil, 2)
	for _, lane := range tl.Lanes {
		for _, st := range lane.Steps {
			if st.End <= st.Start {
				t.Errorf("step %q has a non-positive length", st.Instruction)
			}
		}
	}
	// 青椒肉丝: board 0–2, stove 2–7, hand 7–8.
	want := map[int]int{0: 2, 2: 7, 7: 8}
	for _, lane := range tl.Lanes {
		for _, st := range lane.Steps {
			if st.RecipeID != 1 {
				continue
			}
			if end, ok := want[st.Start]; !ok || end != st.End {
				t.Errorf("dish 1 step %d = %d-%d, want a chained block", st.StepIndex, st.Start, st.End)
			}
		}
	}
}

func TestStepMinutes(t *testing.T) {
	cases := []struct {
		in   *float64
		want int
	}{
		{nil, 1},
		{minutes(0), 1},
		{minutes(-3), 1},
		{minutes(2.4), 2},
		{minutes(2.6), 3},
		{minutes(6000), maxClampMinutes},
	}
	for _, c := range cases {
		if got := StepMinutes(domain.Step{DurationMinutes: c.in}); got != c.want {
			t.Errorf("StepMinutes(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPrimaryResource(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, domain.ResourceHand},
		{[]string{"stove", "waiting"}, domain.ResourceStove},
		{[]string{"board", "hand"}, domain.ResourceBoard},
		{[]string{"waiting"}, domain.ResourceWaiting},
		{[]string{"unknown"}, domain.ResourceHand},
	}
	for _, c := range cases {
		if got := PrimaryResource(c.in); got != c.want {
			t.Errorf("PrimaryResource(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBuildPrepAggregates checks the prep list: alias-normalized names merge,
// amounts add up, coarse stock counts as covered and seasonings are listed
// apart.
func TestBuildPrepAggregates(t *testing.T) {
	recipes := []domain.Recipe{
		{ID: 1, Name: "番茄炒蛋", Ingredients: []domain.Ingredient{
			{Name: "番茄", Amount: 2, Unit: "个", Required: true},
			{Name: "鸡蛋", Amount: 3, Unit: "个", Required: true},
			{Name: "盐", Amount: 2, Unit: "g", Required: true, Seasoning: true},
		}},
		{ID: 2, Name: "番茄汤", Ingredients: []domain.Ingredient{
			{Name: "西红柿", Amount: 1, Unit: "个", Required: true},
			{Name: "盐", Amount: 1, Unit: "g", Required: true, Seasoning: true},
			{Name: "香菜", Amount: 1, Unit: "把", Required: false},
		}},
	}
	items := []domain.Item{
		{ID: 1, Name: "西红柿", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityPrecise, Quantity: float64Ptr(4), Unit: "个"},
		{ID: 2, Name: "鸡蛋", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularSome},
	}
	lines := BuildPrep(recipes, items, cookmatch.MustNew())
	if len(lines) != 3 {
		for _, l := range lines {
			t.Logf("line %+v", l)
		}
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	tomato := findLine(lines, "番茄")
	if tomato == nil {
		t.Fatalf("no merged tomato line: %+v", lines)
	}
	if tomato.Needed != 3 || tomato.Unit != "个" {
		t.Errorf("tomato = %v %s, want 3 个", tomato.Needed, tomato.Unit)
	}
	if !reflect.DeepEqual(tomato.Recipes, []string{"番茄炒蛋", "番茄汤"}) {
		t.Errorf("tomato recipes = %v", tomato.Recipes)
	}
	if !tomato.InStock || tomato.Missing != 0 {
		t.Errorf("tomato stock = %+v, want covered by 4 个", tomato)
	}
	egg := findLine(lines, "鸡蛋")
	if egg == nil || !egg.InStock || egg.Missing != 0 {
		t.Errorf("coarse stock should count as covered: %+v", egg)
	}
	salt := findLine(lines, "盐")
	if salt == nil || !salt.Seasoning || salt.Group != "seasoning" {
		t.Errorf("salt line = %+v", salt)
	}
	if findLine(lines, "香菜") != nil {
		t.Error("optional ingredients must stay out of the prep list")
	}
	groups := GroupPrep(lines)
	if len(groups) != 2 {
		for _, g := range groups {
			t.Logf("group %s %d lines", g.Title, len(g.Lines))
		}
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].Title != "生鲜" || groups[1].Title != "调料" {
		t.Errorf("group titles = %q %q", groups[0].Title, groups[1].Title)
	}
}

// TestPlanDeduction covers the four deduction shapes: precise subtraction,
// coarse step-down, skipping what is not in the pantry, and never touching
// seasonings.
func TestPlanDeduction(t *testing.T) {
	recipes := []domain.Recipe{
		{ID: 1, Name: "青椒肉丝", Ingredients: []domain.Ingredient{
			{Name: "土豆", Amount: 2, Unit: "个", Required: true},
			{Name: "洋葱", Amount: 1, Unit: "个", Required: true},
			{Name: "青椒", Amount: 60, Unit: "g", Required: true},
			{Name: "生抽", Amount: 10, Unit: "ml", Required: true, Seasoning: true},
		}},
		{ID: 2, Name: "土豆丝", Ingredients: []domain.Ingredient{
			{Name: "土豆", Amount: 3, Unit: "个", Required: true},
		}},
	}
	items := []domain.Item{
		{ID: 10, Name: "马铃薯", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityPrecise, Quantity: float64Ptr(4), Unit: "个"},
		{ID: 11, Name: "洋葱", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularPlenty},
		{ID: 12, Name: "生抽", Kind: domain.ItemKindSeasoning,
			QuantityMode: domain.QuantityPrecise, Quantity: float64Ptr(500), Unit: "ml"},
	}
	changes := PlanDeduction(recipes, items, cookmatch.MustNew())

	potato := findChange(changes, "马铃薯")
	if potato == nil || potato.Skipped {
		t.Fatalf("potato change = %+v", potato)
	}
	// 4 - (2 + 3) is below zero: the row is used up.
	if !potato.Precise || potato.Before != 4 || potato.After != 0 || !potato.Depleted {
		t.Errorf("potato = %+v, want used up", potato)
	}
	if len(potato.Recipes) != 2 {
		t.Errorf("potato recipes = %v", potato.Recipes)
	}
	onion := findChange(changes, "洋葱")
	if onion == nil || onion.Precise || onion.BeforeLevel != domain.GranularPlenty ||
		onion.AfterLevel != domain.GranularSome || onion.Depleted {
		t.Errorf("onion = %+v, want 充足 → 还有一些", onion)
	}
	if findChange(changes, "生抽") != nil {
		t.Error("seasoning must not be deducted")
	}
	pepper := findChange(changes, "青椒")
	if pepper == nil || !pepper.Skipped || pepper.Reason != "不在仓库" {
		t.Errorf("pepper = %+v, want skipped as not in pantry", pepper)
	}
}

// TestPlanDeductionLowLevelStepsDownToDepleted covers the last coarse step.
func TestPlanDeductionLowLevelStepsDownToDepleted(t *testing.T) {
	recipes := []domain.Recipe{{ID: 1, Name: "汤", Ingredients: []domain.Ingredient{
		{Name: "香菇", Amount: 5, Unit: "朵", Required: true},
	}}}
	items := []domain.Item{{ID: 3, Name: "香菇", Kind: domain.ItemKindIngredient,
		QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularLow}}
	changes := PlanDeduction(recipes, items, cookmatch.MustNew())
	if len(changes) != 1 {
		t.Fatalf("changes = %+v", changes)
	}
	if !changes[0].Depleted || changes[0].AfterLevel != "" {
		t.Errorf("change = %+v, want used up", changes[0])
	}
}

// TestPlanDeductionUnitMismatch leaves an unconvertible row alone.
func TestPlanDeductionUnitMismatch(t *testing.T) {
	recipes := []domain.Recipe{{ID: 1, Name: "汤", Ingredients: []domain.Ingredient{
		{Name: "牛奶", Amount: 200, Unit: "ml", Required: true},
	}}}
	items := []domain.Item{{ID: 4, Name: "牛奶", Kind: domain.ItemKindIngredient,
		QuantityMode: domain.QuantityPrecise, Quantity: float64Ptr(2), Unit: "盒"}}
	changes := PlanDeduction(recipes, items, cookmatch.MustNew())
	if len(changes) != 1 || !changes[0].Skipped || changes[0].Reason != "单位不同" {
		t.Errorf("changes = %+v", changes)
	}
}

func TestRunLines(t *testing.T) {
	tl := BuildTimeline(goldenRecipes(), goldenSteps(), map[int64]int{3: 4}, 2)
	lines := RunLines(tl)
	if len(lines) != 8 {
		t.Fatalf("lines = %d, want 8", len(lines))
	}
	if lines[0].RecipeName != "青椒肉丝" || lines[0].Text != "青椒切丝" || lines[0].Minutes != 2 {
		t.Errorf("first line = %+v", lines[0])
	}
	// The run follows the timeline, so the second line is another dish's
	// step starting at the same minute, not dish 1's second step.
	if lines[1].RecipeID == lines[0].RecipeID {
		t.Errorf("the run is still dish-by-dish: %+v", lines[:3])
	}
	waiting := 0
	for _, line := range lines {
		if line.Waiting {
			waiting++
		}
	}
	if waiting != 1 {
		t.Errorf("waiting lines = %d, want the braise step", waiting)
	}
}

// TestTimelineStepsFollowStarts pins the flat order the lyric flow walks.
func TestTimelineStepsFollowStarts(t *testing.T) {
	tl := BuildTimeline(goldenRecipes(), goldenSteps(), map[int64]int{3: 4}, 2)
	want := []int{0, 0, 0, 2, 2, 3, 7, 13}
	if len(tl.Steps) != len(want) {
		t.Fatalf("steps = %d, want %d", len(tl.Steps), len(want))
	}
	for i, at := range want {
		if tl.Steps[i].Start != at {
			t.Errorf("step %d starts at %d, want %d", i, tl.Steps[i].Start, at)
		}
	}
}

func float64Ptr(v float64) *float64 { return &v }

func findLine(lines []domain.PrepLine, name string) *domain.PrepLine {
	for i := range lines {
		if lines[i].Name == name {
			return &lines[i]
		}
	}
	return nil
}

func findChange(changes []domain.StockChange, name string) *domain.StockChange {
	for i := range changes {
		if changes[i].Name == name {
			return &changes[i]
		}
	}
	return nil
}
