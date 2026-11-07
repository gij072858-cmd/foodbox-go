package nutrition

// Purine grading for the gout-aware parts of the app.
//
// The table gives mg per 100 g for part of the catalog; the rest is graded by
// keyword. A name neither the table nor the keyword lists know stays
// "未覆盖" — it is never quietly treated as low, because that would make a
// purine budget look better than it is.

import (
	"fmt"
	"strings"
)

// PurineLevel is a food's purine tier.
type PurineLevel string

// The four tiers. Unknown is not low.
const (
	PurineUnknown PurineLevel = "未覆盖"
	PurineLow     PurineLevel = "低"
	PurineMedium  PurineLevel = "中"
	PurineHigh    PurineLevel = "高"
)

// Default thresholds when no appendix is loaded: clinically usual cut-offs
// (低 <50, 中 50–150, 高 ≥150 mg/100 g).
const (
	defaultPurineLowBelow    = 50.0
	defaultPurineMediumBelow = 150.0
)

// AcutePurineCapMg is the daily ceiling for an acute gout flare.
const AcutePurineCapMg = 150.0

// Severity orders the tiers when several ingredients are combined. Unknown
// does not take part.
func (l PurineLevel) Severity() int {
	switch l {
	case PurineLow:
		return 1
	case PurineMedium:
		return 2
	case PurineHigh:
		return 3
	}
	return 0
}

// LevelFromMg grades a per-100 g purine value with the default thresholds.
func LevelFromMg(mg float64) PurineLevel {
	return levelFromMg(mg, defaultPurineLowBelow, defaultPurineMediumBelow)
}

// LevelFor grades one ingredient. The table's number wins, then the row's own
// level string, then the keyword lists.
func (c *Catalog) LevelFor(name string) PurineLevel {
	if c != nil {
		if e, ok := c.Lookup(name); ok {
			if e.PurineMg != nil {
				return levelFromMg(*e.PurineMg, c.purineLow, c.purineMedium)
			}
			if l := PurineLevel(e.PurineLevel); l.Severity() > 0 {
				return l
			}
		}
		if mg, ok := c.purines[Normalize(name)]; ok {
			return levelFromMg(mg, c.purineLow, c.purineMedium)
		}
	}
	return KeywordPurineLevel(name)
}

// DishLevel is the worst tier among a dish's ingredients.
func (c *Catalog) DishLevel(names []string) PurineLevel {
	worst := PurineUnknown
	for _, name := range names {
		if l := c.LevelFor(name); l.Severity() > worst.Severity() {
			worst = l
		}
	}
	return worst
}

// levelFromMg applies the two thresholds.
func levelFromMg(mg, lowBelow, mediumBelow float64) PurineLevel {
	switch {
	case mg < lowBelow:
		return PurineLow
	case mg < mediumBelow:
		return PurineMedium
	default:
		return PurineHigh
	}
}

// kwPurineHigh: offal, concentrated stock, small oily fish, shellfish, beer
// and yeast. An acute gout flare is a hard no on these.
var kwPurineHigh = []string{
	"内脏", "肝", "腰子", "腰花", "脑", "肥肠", "猪肠", "大肠", "牛肚", "毛肚",
	"鸡胗", "鸭胗", "鸡心", "猪心", "鸭肠", "血旺", "鸭血", "猪血",
	"老火汤", "高汤", "浓汤", "肉汤", "骨汤", "火锅汤", "卤汤", "鸡汤", "汤底",
	"沙丁", "凤尾鱼", "鲭鱼", "鲱鱼", "鱼籽", "鱼子", "鱼卵", "带鱼",
	"虾", "虾米", "虾皮", "蟹", "贝", "蛤", "蚬", "扇贝", "生蚝", "牡蛎", "蛏子",
	"花甲", "鲍", "啤酒", "酵母", "鸡精", "浓缩",
}

// kwPurineMedium: everyday meat, most freshwater fish, soy products and part
// of the fungi and algae. The bird words are spelled out (鸡肉, not 鸡) so that
// 鸡蛋 stays low.
var kwPurineMedium = []string{
	"猪", "牛", "羊", "里脊", "排骨", "五花", "肉末", "肉丝", "肉片", "腊肉", "香肠", "火腿",
	"鸡肉", "鸡胸", "鸡翅", "鸡腿", "鸡爪", "鸡脖", "鸭肉", "鸭掌", "鹅肉", "鹌鹑", "乳鸽",
	"鱼", "鲫", "鲤", "鲈", "鳕", "黄鱼", "鲳", "鲅", "罗非", "鳝", "鳗", "龙利",
	"鱿鱼", "章鱼", "墨鱼", "海参",
	"黄豆", "豆腐", "豆干", "豆皮", "腐竹", "千张", "豆浆", "豆豉", "纳豆",
	"香菇", "紫菜", "海带", "裙带菜", "芦笋", "菠菜", "菜花", "西兰花", "蘑菇", "平菇",
	"花生", "腰果", "葵花籽",
}

// kwPurineLow: eggs, dairy, most vegetables and fruit, grains and tubers.
var kwPurineLow = []string{
	"蛋", "牛奶", "酸奶", "奶粉", "奶酪", "芝士", "黄油", "炼乳",
	"米", "面", "馒头", "面包", "粉丝", "粉条", "土豆", "红薯", "山药", "南瓜", "芋头", "玉米",
	"白菜", "生菜", "油麦菜", "娃娃菜", "黄瓜", "冬瓜", "丝瓜", "苦瓜", "茄子", "番茄",
	"西红柿", "青椒", "胡萝卜", "白萝卜", "洋葱", "芹菜", "莲藕", "豆芽", "木耳", "银耳",
	"金针菇", "苹果", "梨", "香蕉", "橙", "柠檬", "葡萄", "西瓜", "桃", "草莓", "枣",
	"食用油", "盐", "糖", "醋", "酱油", "淀粉", "葱", "姜", "蒜",
}

// KeywordPurineLevel grades a name by keyword only. The lists are checked
// high → medium → low, so 猪肝 reads high rather than medium.
func KeywordPurineLevel(name string) PurineLevel {
	switch {
	case containsAny(name, kwPurineHigh...):
		return PurineHigh
	case containsAny(name, kwPurineMedium...):
		return PurineMedium
	case containsAny(name, kwPurineLow...):
		return PurineLow
	}
	return PurineUnknown
}

// PurineNote is the one-line daily purine summary the dashboard shows. mg is
// the sum of the dishes whose ingredients carry purine data; missing counts
// the dishes that were left out because the table has no number for them.
func PurineNote(mg float64, missing int, acute bool) string {
	if mg <= 0 && missing == 0 {
		return "今天还没有开饭记录"
	}
	if mg <= 0 {
		return "今天记录的菜都没有嘌呤数据，暂不能估算"
	}
	if acute {
		if mg > AcutePurineCapMg {
			return fmt.Sprintf("今日嘌呤约 %d mg（估算），已超急性期上限 %d mg",
				intRound(mg), intRound(AcutePurineCapMg))
		}
		return fmt.Sprintf("今日嘌呤约 %d mg（估算），急性期上限 %d mg，还可约 %d mg",
			intRound(mg), intRound(AcutePurineCapMg), intRound(AcutePurineCapMg-mg))
	}
	note := fmt.Sprintf("今日嘌呤约 %d mg（估算），缓解期不设硬上限，供参考", intRound(mg))
	if missing > 0 {
		note = strings.TrimSuffix(note, "，供参考") +
			fmt.Sprintf("；另有 %d 道菜缺嘌呤数据未计入", missing)
	}
	return note
}

// intRound rounds to the nearest integer for display.
func intRound(v float64) int {
	if v < 0 {
		return -intRound(-v)
	}
	return int(v + 0.5)
}
