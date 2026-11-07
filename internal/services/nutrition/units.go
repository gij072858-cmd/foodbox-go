package nutrition

// Unit conversion for recipe ingredient lines.
//
// The shipped recipe data already carries a gram weight per line, but the
// editor and imported recipes do not, so the estimator has to derive one from
// "amount + unit". The table below is a set of household measures, not a lab
// measurement: every caller that leans on it must say 估算 next to the number.
//
// Unknown units never become a silent zero: GramsFor reports ok=false and the
// catalog falls back to a typical portion while marking the estimate partial.

import (
	"strings"
)

// GramsFor converts amount × unit into an edible-portion weight in grams.
//
// It returns ok=false when the unit is not a measure this table knows; the
// caller then falls back to TypicalGrams and reports a partial estimate.
// A bare number with no unit is taken as grams, which is how the recipe
// editor writes quick entries.
func GramsFor(name string, amount float64, unit string) (grams float64, ok bool) {
	if amount <= 0 {
		return 0, false
	}
	if strings.TrimSpace(unit) == "" {
		return amount, true
	}
	u, known := canonicalUnit(unit)
	if !known {
		return 0, false
	}
	switch u {
	case "g":
		return amount, true
	case "kg":
		return amount * 1000, true
	case "jin":
		return amount * 500, true
	case "liang":
		return amount * 50, true
	case "ml":
		return amount, true
	case "l":
		return amount * 1000, true
	case "cw":
		return amount * 200, true
	case "cup":
		return amount * 240, true
	case "tsp":
		return amount * 3, true
	case "tbsp":
		return amount * spoonGrams(name), true
	case "piece":
		return amount * perPieceGrams(name), true
	case "root":
		return amount * perRootGrams(name), true
	case "slice":
		return amount * 15, true
	case "chunk":
		return amount * 30, true
	case "strip":
		return amount * 40, true
	case "clove":
		return amount * 5, true
	case "head":
		return amount * 40, true
	case "floret":
		return amount * 15, true
	case "handful":
		return amount * 100, true
	case "grain":
		return amount * maxFloat(perPieceGrams(name)/3, 2), true
	case "pellet":
		return amount * 2, true
	case "stretch":
		return amount * 30, true
	case "sheet":
		return amount * 15, true
	case "skewer":
		return amount * 60, true
	case "stick":
		return amount * 30, true
	case "pinch":
		return amount * 2, true
	case "drop":
		return amount * 1, true
	}
	return 0, false
}

// unitAliases maps the spellings recipes use onto the small canonical set the
// switch above works with.
var unitAliases = map[string]string{
	"":      "",
	"克":     "g",
	"公克":    "g",
	"g":     "g",
	"gram":  "g",
	"grams": "g",

	"kg": "kg", "千克": "kg", "公斤": "kg", "kilogram": "kg",
	"斤": "jin",
	"两": "liang",

	"ml": "ml", "毫升": "ml", "cc": "ml",
	"l": "l", "升": "l", "liter": "l", "litre": "l",

	"个": "piece", "只": "piece", "支": "piece", "枚": "piece",
	"根": "root", "条": "strip", "片": "slice", "块": "chunk",
	"瓣": "clove", "头": "head", "朵": "floret",
	"颗": "grain", "粒": "pellet",
	"把": "handful", "扎": "handful", "棵": "handful",
	"段": "stretch", "张": "sheet", "串": "skewer",
	"勺": "tbsp", "大勺": "tbsp", "汤勺": "tbsp", "汤匙": "tbsp", "满勺": "tbsp",
	"茶匙": "tsp", "小勺": "tsp", "茶勺": "tsp",
	"碗": "cw", "杯": "cup",
	"撮": "pinch", "滴": "drop",
}

// canonicalUnit maps a unit spelling onto the small canonical set the switch
// above works with. It reports known=false for anything unmapped, including
// container units such as 罐 / 包 / 袋: a can of tomatoes is not a weight, so
// the caller falls back to a portion estimate instead of inventing one.
func canonicalUnit(unit string) (string, bool) {
	u := strings.ToLower(strings.TrimSpace(unit))
	u = strings.ReplaceAll(u, " ", "")
	if v, ok := unitAliases[u]; ok {
		return v, true
	}
	return "", false
}

// pieceOrder is the ingredient table for 个 / 只, longest names first so
// 洋葱 is not eaten by 葱.
var pieceOrder = []struct {
	match string
	grams float64
}{
	{"娃娃菜", 200}, {"大白菜", 500}, {"小白菜", 150},
	{"西兰花", 300}, {"胡萝卜", 100}, {"白萝卜", 300},
	{"西红柿", 150}, {"小米椒", 5}, {"干辣椒", 2},
	{"杏鲍菇", 60}, {"金针菇", 50}, {"饺子皮", 10}, {"馄饨皮", 8},
	{"鸡腿", 120}, {"鸡翅", 60}, {"番茄", 150}, {"土豆", 150},
	{"洋葱", 200}, {"青椒", 60}, {"尖椒", 30}, {"茄子", 200},
	{"黄瓜", 150}, {"冬瓜", 500}, {"南瓜", 300}, {"玉米", 200},
	{"山药", 200}, {"莲藕", 200}, {"菜花", 300}, {"生菜", 150},
	{"苹果", 200}, {"香蕉", 120}, {"柠檬", 100}, {"香菇", 20},
	{"馒头", 100}, {"包子", 80}, {"面包", 50}, {"豆腐", 200},
	{"鸡蛋", 50}, {"鸭蛋", 60}, {"皮蛋", 60}, {"虾", 12},
	{"梨", 200}, {"橙子", 200},
}

// perPieceGrams is how much one 个 / 只 of an ingredient weighs. Unmatched
// names get a generic portion, which the estimator reports as a guess.
func perPieceGrams(name string) float64 {
	for _, row := range pieceOrder {
		if strings.Contains(name, row.match) {
			return row.grams
		}
	}
	return 120
}

// perRootGrams is one 根: spring onions and small aromatics are far lighter
// than a root vegetable.
func perRootGrams(name string) float64 {
	for _, kw := range []string{"葱", "香菜", "韭菜", "蒜苗", "菠菜", "芹菜"} {
		if strings.Contains(name, kw) {
			return 40
		}
	}
	for _, kw := range []string{"胡萝卜", "黄瓜", "茄子", "西葫芦"} {
		if strings.Contains(name, kw) {
			return 100
		}
	}
	for _, kw := range []string{"山药", "白萝卜", "莲藕", "玉米"} {
		if strings.Contains(name, kw) {
			return 180
		}
	}
	return 80
}

// spoonGrams is how heavy one 勺 / 汤匙 of an ingredient is: oil floats, pastes
// are dense, and soy sauce sits between them.
func spoonGrams(name string) float64 {
	switch {
	case containsAny(name, "油", "香油", "麻油", "花椒油", "橄榄"):
		return 10
	case containsAny(name, "酱", "豆瓣", "蚝油", "豆豉", "腐乳", "番茄沙司", "沙拉"):
		return 15
	case containsAny(name, "糖", "盐", "淀粉", "味精", "鸡精", "胡椒", "花椒", "孜然", "五香", "十三香", "咖喱"):
		return 10
	case containsAny(name, "生抽", "老抽", "酱油", "料酒", "醋", "黄酒", "味淋", "蜂蜜"):
		return 12
	}
	return 12
}

// typicalWeight keeps the keyword table in one place; entries are checked in
// order, so the specific cases come before the generic ones.
type typicalWeight struct {
	keywords []string
	grams    float64
}

// typicalTable is the fallback portion for a line with no usable amount. The
// order is the priority: a name that contains 葱 must not be read as a
// vegetable portion if the name is 洋葱, hence the targeted rows first.
var typicalTable = []typicalWeight{
	// Ingredients that contain another category's word.
	{[]string{"牛油果"}, 150},
	{[]string{"猪油渣"}, 20},
	{[]string{"猪油", "鸡油", "鸭油", "牛油", "荤油"}, 15},
	{[]string{"虾皮", "海米", "虾米"}, 8},
	{[]string{"花生米"}, 20},
	{[]string{"干香菇", "干木耳", "干蘑菇", "香菇干", "木耳干"}, 10},
	{[]string{"小米辣", "小米椒", "朝天椒"}, 5},
	{[]string{"洋葱"}, 150},
	{[]string{"枸杞", "党参", "当归", "黄芪"}, 10},
	{[]string{"紫苏", "九层塔", "香茅", "薄荷"}, 10},

	// Seasonings: the ones that decide whether a dish reads as light or heavy.
	{[]string{"食用油", "植物油", "花生油", "玉米油", "菜籽油", "橄榄油"}, 15},
	{[]string{"香油", "麻油", "花椒油"}, 5},
	{[]string{"盐"}, 3},
	{[]string{"白糖", "冰糖", "砂糖", "红糖", "黑糖"}, 8},
	{[]string{"生抽", "味极鲜"}, 10},
	{[]string{"老抽"}, 6},
	{[]string{"酱油", "鱼露", "蒸鱼豉油", "白灼汁"}, 10},
	{[]string{"蚝油"}, 10},
	{[]string{"料酒", "黄酒", "花雕", "米酒", "醪糟", "味淋", "清酒", "啤酒", "葡萄酒", "白酒"}, 10},
	{[]string{"醋"}, 8},
	{[]string{"味精", "鸡精", "鸡汁", "鸡粉", "牛肉粉"}, 2},
	{[]string{"辣椒粉", "红椒粉", "泡打粉", "酵母", "咖喱粉", "奥尔良粉", "香菜籽粉", "五香粉", "十三香"}, 3},
	{[]string{"淀粉", "生粉"}, 8},
	{[]string{"豆瓣", "甜面酱", "黄豆酱", "老干妈", "辣椒酱"}, 15},
	{[]string{"番茄酱", "番茄沙司"}, 15},
	{[]string{"胡椒", "花椒", "孜然", "辣椒面", "干辣椒", "辣椒段"}, 3},
	{[]string{"蒜"}, 8},
	{[]string{"姜"}, 5},
	{[]string{"葱"}, 10},
	{[]string{"香菜", "罗勒"}, 5},
	{[]string{"蜂蜜"}, 8},

	// Main ingredients: one household portion.
	{[]string{"鸡蛋", "鸭蛋", "鹅蛋", "鹌鹑蛋"}, 100},
	{[]string{"米", "面", "粉", "馒头", "面包", "年糕", "燕麦", "粉丝", "粉条"}, 80},
	{[]string{"排骨", "五花", "里脊", "肉末", "肉丝", "肉片"}, 150},
	{[]string{"肉", "牛", "羊", "猪", "鸡", "鸭", "鹅"}, 150},
	{[]string{"鱼", "虾", "蟹", "贝", "蛤", "鱿", "蚝", "螺"}, 150},
	{[]string{"蛋"}, 50},
	{[]string{"豆腐", "豆干", "腐竹", "豆皮", "千张"}, 150},
	{[]string{"奶", "芝士", "黄油", "奶油"}, 30},
	{[]string{"木耳", "香菇", "菇", "菌"}, 100},
	{[]string{"花生", "腰果", "核桃", "芝麻", "松子"}, 20},
}

// TypicalGrams is the fallback weight for a line that has no usable amount.
// It is a rough portion, never a measurement, and the caller marks the
// estimate partial.
func TypicalGrams(name string, seasoning bool) float64 {
	for _, row := range typicalTable {
		if containsAny(name, row.keywords...) {
			return row.grams
		}
	}
	if seasoning {
		return 8
	}
	return 150
}

// containsAny reports whether name contains any of the substrings.
func containsAny(name string, subs ...string) bool {
	for _, s := range subs {
		if s != "" && strings.Contains(name, s) {
			return true
		}
	}
	return false
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
