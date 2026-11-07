// Package sichu plans an offline home meal from the recipe library.
//
// The planner is deliberately deterministic: the same input and the same day
// always produce the same plan, because the whole point is a trustworthy
// default when there is no network or no API key. It is also conservative: a
// dish that breaks a diner's declared limits never enters the pool, and a plan
// that breaks the meal structure is rejected whole rather than patched by
// dropping a dish.
//
// Hard limits come from the same vocabulary the Flutter app wrote to the
// database (enum names in comma-separated text), so old profiles keep working.
package sichu

import (
	"regexp"
	"strings"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// Role is a dish's nutrition shape, folded from its main-ingredient category.
type Role string

const (
	RoleProtein Role = "protein"
	RoleVeg     Role = "veg"
	RoleStaple  Role = "staple"
	RoleUnknown Role = "unknown"
)

// proteinCategories are the main categories that fill the main-dish slot.
// The values are the seed's mainCategory strings.
var proteinCategories = map[string]bool{
	"猪肉": true, "牛肉": true, "羊肉": true, "鸡肉": true, "鸭肉": true,
	"鱼类": true, "虾蟹贝": true, "其他海鲜": true, "蛋类": true, "豆制品": true,
}

// vegCategories fill the vegetable slot.
var vegCategories = map[string]bool{"蔬菜": true, "菌菇": true}

// stapleCategories are staple dishes: they never occupy a slot of their own.
var stapleCategories = map[string]bool{"主食": true, "面点": true}

// soupCuisine is the only cuisine that makes a soup-course dish. A dish whose
// name merely ends in 汤 does not count: the rule is about the cooking method.
const soupCuisine = "汤羹"

// RoleOfCategory folds a main-ingredient category into its nutrition role.
func RoleOfCategory(category string) Role {
	cat := strings.TrimSpace(category)
	switch {
	case stapleCategories[cat]:
		return RoleStaple
	case proteinCategories[cat]:
		return RoleProtein
	case vegCategories[cat]:
		return RoleVeg
	default:
		return RoleUnknown
	}
}

// RoleOf folds a recipe's main category into its nutrition role.
func RoleOf(rec domain.Recipe) Role {
	return RoleOfCategory(rec.MainCategory)
}

// IsSoup reports whether a recipe is a soup course.
func IsSoup(rec domain.Recipe) bool {
	return strings.TrimSpace(rec.Cuisine) == soupCuisine
}

// oneBowlKeywords are the names that count as "one bowl, all in": the dish
// already carries staple, protein and vegetable, so it is the whole meal.
var oneBowlKeywords = []string{
	"盖饭", "拌饭", "焖饭", "煲仔饭", "手抓饭", "焗饭", "炒饭",
	"炒面", "炒粉", "拌面", "拌粉", "烩面", "焖面", "汤面", "拉面",
	"刀削面", "乌冬", "意面", "米线", "米粉", "河粉", "粥", "饭包",
	"肉饼", "糊塌子", "一锅出",
}

// IsOneBowlName reports whether a dish name looks like a one-bowl meal.
func IsOneBowlName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, kw := range oneBowlKeywords {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

// versionSuffix matches a trailing full-width parenthetical, the same rule the
// seed's taxonomy uses to group variants such as 口水鸡（川味）.
var versionSuffix = regexp.MustCompile(`^(.*\S)（[^（）]*）\s*$`)

// NormalizeVersionGroup strips a trailing full-width parenthetical from a
// dish name, falling back to the trimmed name when nothing is left.
func NormalizeVersionGroup(name string) string {
	name = strings.TrimSpace(name)
	if m := versionSuffix.FindStringSubmatch(name); m != nil {
		if stripped := strings.TrimSpace(m[1]); stripped != "" {
			return stripped
		}
	}
	return name
}

// VersionGroup returns the stored version group, or the name-derived one for
// rows written before the column existed.
func VersionGroup(rec domain.Recipe) string {
	if stored := strings.TrimSpace(rec.VersionGroup); stored != "" {
		return stored
	}
	return NormalizeVersionGroup(rec.Name)
}

// Slot is a place in one meal's structure.
type Slot string

const (
	SlotMain Slot = "main"
	SlotVeg  Slot = "veg"
	SlotSoup Slot = "soup"
	SlotCore Slot = "core"
)

// Label is the slot's Chinese label for the result page.
func (s Slot) Label() string {
	switch s {
	case SlotMain:
		return "主菜"
	case SlotVeg:
		return "蔬菜类菜"
	case SlotSoup:
		return "汤"
	case SlotCore:
		return "一碗端"
	default:
		return "配菜"
	}
}

// Shape describes how one dish sits in a meal structure.
type Shape struct {
	Role    Role
	Soup    bool
	OneBowl bool
}

// ShapeOf derives the structure of a library recipe.
func ShapeOf(rec domain.Recipe) Shape {
	return Shape{Role: RoleOf(rec), Soup: IsSoup(rec), OneBowl: IsOneBowlName(rec.Name)}
}
