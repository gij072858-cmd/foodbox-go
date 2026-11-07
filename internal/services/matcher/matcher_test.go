package matcher_test

import (
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
)

func ing(name string, required bool) domain.Ingredient {
	return domain.Ingredient{Name: name, Required: required, Amount: 1}
}

func item(name string, depleted bool) domain.Item {
	return domain.Item{Name: name, IsDepleted: depleted}
}

func recipe(id int64, ingredients ...domain.Ingredient) domain.Recipe {
	return domain.Recipe{ID: id, Name: "测试菜", Ingredients: ingredients}
}

func TestAliasAndThreeStates(t *testing.T) {
	m := matcher.MustNew()

	// 西红柿 is an alias of 番茄; a depleted row still counts (v1 contract).
	r := recipe(1, ing("番茄", true), ing("鸡蛋", true), ing("盐", true))
	res := m.Match(r, []domain.Item{item("西红柿", false), item("鸡蛋", true), item("盐", false)})
	if res.Status != matcher.CanCook {
		t.Errorf("status = %s, missing %v", res.Status, res.Missing)
	}

	// One and two missing required items are "almost".
	r = recipe(2, ing("猪肉", true), ing("青椒", true), ing("蒜", true), ing("酱油", false))
	res = m.Match(r, []domain.Item{item("猪肉", false)})
	if res.Status != matcher.Almost || len(res.Missing) != 2 {
		t.Errorf("status = %s, missing = %d", res.Status, len(res.Missing))
	}
	if got := res.MissingLabel(); got != "缺：青椒、蒜" {
		t.Errorf("missing label = %q", got)
	}

	// Three missing is far away, and optional items do not affect status.
	r = recipe(3, ing("牛肉", true), ing("土豆", true), ing("胡萝卜", true), ing("香菜", false))
	res = m.Match(r, nil)
	if res.Status != matcher.FarAway || len(res.Missing) != 3 {
		t.Errorf("status = %s, missing = %d", res.Status, len(res.Missing))
	}

	// Non-required entries never decide the state.
	r = recipe(4, ing("鸡蛋", false))
	if res := m.Match(r, nil); res.Status != matcher.CanCook {
		t.Errorf("optional-only recipe = %s", res.Status)
	}
}

func TestMatchAllBuildsStockOnce(t *testing.T) {
	m := matcher.MustNew()
	recipes := make([]domain.Recipe, 200)
	items := make([]domain.Item, 0, 200)
	for i := 0; i < 200; i++ {
		recipes[i] = recipe(int64(i+1), ing("食材A", true), ing("食材B", true))
		items = append(items, item("食材A", false))
	}
	results := m.MatchAll(recipes, items)
	if len(results) != len(recipes) {
		t.Fatalf("results = %d", len(results))
	}
	for _, res := range results {
		if res.Status != matcher.Almost || len(res.Missing) != 1 {
			t.Fatalf("result = %s / %d", res.Status, len(res.Missing))
		}
	}
}
