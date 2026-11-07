package sichu

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// ErrEmptyPlan is what Validate returns for a plan without meals.
var ErrEmptyPlan = errors.New("配餐结果为空")

// PlanError names the meal and the reason a plan was rejected. A rejected
// meal is not patched: the whole answer goes back, exactly like the rules
// file demands.
type PlanError struct {
	Meal   string
	Reason string
}

func (e *PlanError) Error() string {
	if e.Meal == "" {
		return e.Reason
	}
	return e.Meal + "：" + e.Reason
}

// Validate checks a plan against the structure contract and every hard limit.
//
// It never modifies its input: callers either fall back to the local plan or
// ask the model again, they never drop a dish to make an invalid meal pass.
func Validate(plan MealPlan, in PlanInput) error {
	in = in.normalize()
	if len(plan.Meals) == 0 {
		return ErrEmptyPlan
	}
	byID := recipeIndex(in.Recipes)
	prefs := in.Preferences()

	seenIDs := make(map[int64]bool)
	seenNames := make(map[string]bool)

	for _, meal := range plan.Meals {
		style := meal.Style
		if style == "" || in.Style == StyleCustom {
			// A custom plan is judged against the counts the diner chose,
			// whatever style string the model wrote back.
			style = in.Style
		}
		// Check that every cited recipe exists before judging the layout: an
		// invented id cannot be classified, so name the real problem first.
		for _, dish := range meal.Dishes {
			if dish.RecipeID == 0 {
				continue
			}
			rec, ok := byID[dish.RecipeID]
			if !ok {
				return &PlanError{Meal: meal.Title, Reason: "「" + strings.TrimSpace(dish.Name) + "」不在菜式库里"}
			}
			if !rec.IsDish {
				return &PlanError{Meal: meal.Title, Reason: "「" + rec.Name + "」不是菜式"}
			}
		}
		if err := validateStructure(meal, style, byID, in); err != nil {
			return err
		}
		for _, dish := range meal.Dishes {
			name := strings.TrimSpace(dish.Name)
			if name == "" {
				return &PlanError{Meal: meal.Title, Reason: "有一道菜没有名字"}
			}
			group := dish.VersionGroup
			if group == "" {
				group = NormalizeVersionGroup(name)
			}
			if seenNames[group] {
				return &PlanError{Meal: meal.Title, Reason: "「" + name + "」重复了"}
			}
			seenNames[group] = true
			if dish.RecipeID != 0 {
				if seenIDs[dish.RecipeID] {
					return &PlanError{Meal: meal.Title, Reason: "「" + name + "」重复了"}
				}
				seenIDs[dish.RecipeID] = true
				rec, ok := byID[dish.RecipeID]
				if !ok {
					return &PlanError{Meal: meal.Title, Reason: "「" + name + "」不在菜式库里"}
				}
				if !rec.IsDish {
					return &PlanError{Meal: meal.Title, Reason: "「" + name + "」不是菜式"}
				}
				if in.ExcludeIDs[rec.ID] || in.ExcludeGroups[VersionGroup(rec)] {
					return &PlanError{Meal: meal.Title, Reason: "「" + name + "」上一版已经出现过"}
				}
				if reason := prefs.DishViolation(rec.Name, RecipeIngredientNames(rec)); reason != "" {
					return &PlanError{Meal: meal.Title, Reason: "「" + rec.Name + "」" + reason}
				}
				if in.MaxMinutes > 0 && rec.DurationMinutes != nil &&
					int(*rec.DurationMinutes) > in.MaxMinutes {
					return &PlanError{Meal: meal.Title, Reason: "「" + rec.Name + "」超过时长上限"}
				}
			} else {
				if dish.Draft == nil {
					return &PlanError{Meal: meal.Title, Reason: "「" + name + "」既不在库里也没有做法"}
				}
				if err := validateDraft(*dish.Draft, name); err != nil {
					return &PlanError{Meal: meal.Title, Reason: err.Error()}
				}
				if reason := prefs.DishViolation(name, dish.Draft.IngredientNames()); reason != "" {
					return &PlanError{Meal: meal.Title, Reason: "「" + name + "」" + reason}
				}
			}
		}
	}
	return nil
}

// validateStructure checks the slot layout of one meal.
func validateStructure(meal Meal, style Style, byID map[int64]domain.Recipe, in PlanInput) error {
	dishes := meal.Dishes
	if style == StyleCustom {
		want := in.Counts
		if want.Total() == 0 {
			return &PlanError{Meal: meal.Title, Reason: "自由配餐至少要选一道菜"}
		}
		var soup, protein, veg, staple int
		for _, dish := range dishes {
			shape := dishShape(dish, byID)
			switch {
			case shape.Soup:
				soup++
			case shape.Role == RoleProtein:
				protein++
			case shape.Role == RoleVeg:
				veg++
			case shape.Role == RoleStaple:
				staple++
			}
		}
		switch {
		case staple > 0:
			return &PlanError{Meal: meal.Title, Reason: "主食不单独占槽位"}
		case protein != want.Mains:
			return &PlanError{Meal: meal.Title, Reason: fmt.Sprintf("自由配餐要 %d 道荤菜，实际 %d 道", want.Mains, protein)}
		case veg != want.Vegs:
			return &PlanError{Meal: meal.Title, Reason: fmt.Sprintf("自由配餐要 %d 道蔬菜类菜，实际 %d 道", want.Vegs, veg)}
		case soup != want.Soups:
			return &PlanError{Meal: meal.Title, Reason: fmt.Sprintf("自由配餐要 %d 道汤羹，实际 %d 道", want.Soups, soup)}
		}
		return nil
	}
	if style == StyleOneBowl {
		if len(dishes) < 1 || len(dishes) > 2 {
			return &PlanError{Meal: meal.Title, Reason: "一碗端要 1 道核心，最多再加 1 道汤"}
		}
		core := dishShape(dishes[0], byID)
		if !core.OneBowl || core.Soup {
			return &PlanError{Meal: meal.Title, Reason: "一碗端的第一道要是自足的盖饭、面或粥"}
		}
		if len(dishes) == 2 {
			if !dishShape(dishes[1], byID).Soup {
				return &PlanError{Meal: meal.Title, Reason: "一碗端只可再配一道汤羹"}
			}
		}
		return nil
	}
	if len(dishes) != 3 {
		return &PlanError{Meal: meal.Title, Reason: "两菜一汤要正好 3 道"}
	}
	var soup, protein, veg, staple int
	for _, dish := range dishes {
		shape := dishShape(dish, byID)
		switch {
		case shape.Soup:
			soup++
		case shape.Role == RoleProtein:
			protein++
		case shape.Role == RoleVeg:
			veg++
		case shape.Role == RoleStaple:
			staple++
		}
	}
	switch {
	case soup != 1:
		return &PlanError{Meal: meal.Title, Reason: "两菜一汤要正好一道汤羹"}
	case protein != 1:
		return &PlanError{Meal: meal.Title, Reason: "两菜一汤要正好一道荤菜"}
	case veg != 1:
		return &PlanError{Meal: meal.Title, Reason: "两菜一汤要正好一道蔬菜类菜"}
	case staple > 0:
		return &PlanError{Meal: meal.Title, Reason: "主食不单独占槽位"}
	}
	return nil
}

// validateDraft enforces the rules file's requirements for a proposed dish.
func validateDraft(d Draft, name string) error {
	if strings.TrimSpace(d.Name) == "" && strings.TrimSpace(name) == "" {
		return errors.New("新菜缺少名字")
	}
	if len(d.Ingredients) == 0 {
		return errors.New("新菜缺少食材清单")
	}
	if len(d.Steps) == 0 {
		return errors.New("新菜缺少步骤")
	}
	for _, step := range d.Steps {
		if strings.TrimSpace(step.Instruction) == "" {
			return errors.New("新菜有空白步骤")
		}
	}
	if d.DurationMinutes < 0 {
		return errors.New("新菜时长不能为负")
	}
	return nil
}

// Describe returns one short line about what a plan contains, for the result
// page's summary.
func Describe(plan MealPlan) string {
	dishes := 0
	minutes := 0
	for _, meal := range plan.Meals {
		dishes += len(meal.Dishes)
		for _, dish := range meal.Dishes {
			minutes += dish.DurationMinutes
		}
	}
	if len(plan.Meals) == 0 {
		return "还没有配餐"
	}
	return fmt.Sprintf("%d 餐 · %d 道菜 · 约 %d 分钟", len(plan.Meals), dishes, minutes)
}
