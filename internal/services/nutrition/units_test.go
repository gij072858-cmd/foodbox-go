package nutrition

import (
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// ingredient builds one recipe line for the grammar tests.
func ingredient(name string, amount float64, unit string, grams *float64) domain.Ingredient {
	return domain.Ingredient{Name: name, Amount: amount, Unit: unit, Grams: grams}
}

// The household measures the app relies on. These are estimates, but they are
// the same estimates every time, which is what the aggregation needs.
func TestGramsFor(t *testing.T) {
	cases := []struct {
		name   string
		amount float64
		unit   string
		want   float64
	}{
		{"鸡蛋", 1, "个", 50},
		{"鸡蛋", 2, "个", 100},
		{"番茄", 1, "个", 150},
		{"番茄", 2, "只", 300},
		{"土豆", 0.5, "kg", 500},
		{"土豆", 1, "斤", 500},
		{"土豆", 2, "两", 100},
		{"大米", 1, "l", 1000},
		{"生抽", 10, "ml", 10},
		{"牛奶", 1, "杯", 240},
		{"米饭", 1, "碗", 200},
		{"食用油", 1, "勺", 10},
		{"生抽", 1, "汤匙", 12},
		{"盐", 1, "茶匙", 3},
		{"盐", 1, "小勺", 3},
		{"蒜", 3, "瓣", 15},
		{"葱", 1, "根", 40},
		{"黄瓜", 1, "根", 100},
		{"豆腐", 1, "块", 30},
		{"带鱼", 1, "条", 40},
		{"姜", 1, "片", 15},
		{"香菇", 1, "朵", 15},
		{"大蒜", 1, "头", 40},
		{"干辣椒", 3, "颗", 6}, // 干辣椒 pieces are tiny: max(2/3, 2)
		{"大米", 1, "粒", 2},
		{"葱", 1, "把", 100},
		{"猪五花", 200, "", 200},
		{"鸡蛋", 1, " KG ", 1000},
	}
	for _, tc := range cases {
		got, ok := GramsFor(tc.name, tc.amount, tc.unit)
		if !ok {
			t.Errorf("GramsFor(%q, %v, %q) not convertible", tc.name, tc.amount, tc.unit)
			continue
		}
		if got != tc.want {
			t.Errorf("GramsFor(%q, %v, %q) = %v, want %v", tc.name, tc.amount, tc.unit, got, tc.want)
		}
	}
}

// Units the table cannot weigh are reported, never guessed as grams.
func TestGramsForUnknownUnit(t *testing.T) {
	for _, unit := range []string{"罐", "包", "袋", "盒", "瓶", "份", "盘", "扎半"} {
		if got, ok := GramsFor("番茄", 1, unit); ok {
			t.Errorf("GramsFor(番茄, 1, %q) = %v, want not convertible", unit, got)
		}
	}
	if _, ok := GramsFor("番茄", 0, "个"); ok {
		t.Error("zero amount should not convert")
	}
}

// The fallback portion follows the ingredient, in priority order: 洋葱 is a
// vegetable, not a spoon of 葱; 小米辣 is a chili, not a bowl of 米.
func TestTypicalGrams(t *testing.T) {
	cases := []struct {
		name      string
		seasoning bool
		want      float64
	}{
		{"洋葱", false, 150},
		{"小米辣", false, 5},
		{"猪油", false, 15},
		{"鸡蛋", false, 100},
		{"盐", true, 3},
		{"食用油", true, 15},
		{"表里没有的食材", false, 150},
		{"表里没有的调料", true, 8},
	}
	for _, tc := range cases {
		if got := TypicalGrams(tc.name, tc.seasoning); got != tc.want {
			t.Errorf("TypicalGrams(%q, %v) = %v, want %v", tc.name, tc.seasoning, got, tc.want)
		}
	}
}

// IngredientGrams prefers the stored gram column, then amount × unit, then a
// typical portion.
func TestIngredientGrams(t *testing.T) {
	stored := 120.0
	if g, inferred, unknown := IngredientGrams(ingredient("猪肉", 200, "g", &stored)); g != 120 || inferred || unknown {
		t.Errorf("stored grams = %v/%v/%v, want 120/false/false", g, inferred, unknown)
	}
	if g, inferred, unknown := IngredientGrams(ingredient("鸡蛋", 2, "个", nil)); g != 100 || inferred || unknown {
		t.Errorf("unit grams = %v/%v/%v, want 100/false/false", g, inferred, unknown)
	}
	if g, inferred, unknown := IngredientGrams(ingredient("鸡蛋", 1, "罐", nil)); g != 100 || !inferred || !unknown {
		t.Errorf("unknown unit = %v/%v/%v, want 100/true/true", g, inferred, unknown)
	}
	if g, inferred, _ := IngredientGrams(ingredient("鸡蛋", 0, "", nil)); g != 100 || !inferred {
		t.Errorf("missing amount = %v/%v, want a typical portion, inferred", g, inferred)
	}
}
