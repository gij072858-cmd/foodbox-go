package nutrition

import (
	"strings"
	"testing"
)

// The clinical cut-offs: 低 <50, 中 50–150, 高 ≥150 mg/100 g.
func TestLevelFromMg(t *testing.T) {
	cases := []struct {
		mg   float64
		want PurineLevel
	}{
		{0, PurineLow},
		{49.9, PurineLow},
		{50, PurineMedium},
		{149.9, PurineMedium},
		{150, PurineHigh},
		{415, PurineHigh},
	}
	for _, tc := range cases {
		if got := LevelFromMg(tc.mg); got != tc.want {
			t.Errorf("LevelFromMg(%v) = %q, want %q", tc.mg, got, tc.want)
		}
	}
}

// The table's number wins over keywords; the appendix fills the gaps; a name
// nothing knows stays unknown rather than low.
func TestCatalogLevelFor(t *testing.T) {
	c := testCatalog(t)
	cases := []struct {
		name string
		want PurineLevel
	}{
		{"紫菜", PurineHigh},    // 415 mg in the table
		{"海苔", PurineLow},     // the table says 17, the appendix says 249: the table wins
		{"菜花", PurineLow},     // not an entry, appendix 41
		{"鸡胸肉", PurineMedium}, // no purine value, keyword fallback
		{"猪肝", PurineHigh},    // offal
		{"表里没有的食材", PurineUnknown},
	}
	for _, tc := range cases {
		if got := c.LevelFor(tc.name); got != tc.want {
			t.Errorf("LevelFor(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A dish takes its worst ingredient tier.
func TestDishLevel(t *testing.T) {
	c := testCatalog(t)
	if got := c.DishLevel([]string{"番茄", "鸡蛋"}); got != PurineLow {
		t.Errorf("DishLevel(番茄+鸡蛋) = %q, want 低", got)
	}
	if got := c.DishLevel([]string{"番茄", "猪肝"}); got != PurineHigh {
		t.Errorf("DishLevel(番茄+猪肝) = %q, want 高", got)
	}
	if got := c.DishLevel(nil); got != PurineUnknown {
		t.Errorf("DishLevel(nil) = %q, want unknown", got)
	}
}

// Keyword grading is the fallback for foods the table has no number for.
func TestKeywordPurineLevel(t *testing.T) {
	cases := []struct {
		name string
		want PurineLevel
	}{
		{"猪肝", PurineHigh},
		{"浓汤", PurineHigh},
		{"排骨", PurineMedium},
		{"豆腐", PurineMedium},
		{"鸡蛋", PurineLow},
		{"番茄", PurineLow},
		{"火鸡面", PurineLow}, // 「面」归到主食，不是未知
		{"未知食材", PurineUnknown},
	}
	for _, tc := range cases {
		if got := KeywordPurineLevel(tc.name); got != tc.want {
			t.Errorf("KeywordPurineLevel(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The daily purine line: honest about missing dishes, strict in a flare.
func TestPurineNote(t *testing.T) {
	if got := PurineNote(0, 0, false); !strings.Contains(got, "还没有开饭记录") {
		t.Errorf("empty note = %q", got)
	}
	if got := PurineNote(0, 2, false); !strings.Contains(got, "暂不能估算") {
		t.Errorf("no data note = %q", got)
	}
	if got := PurineNote(120, 0, false); !strings.Contains(got, "120") || !strings.Contains(got, "缓解期") {
		t.Errorf("normal note = %q", got)
	}
	if got := PurineNote(120, 2, false); !strings.Contains(got, "另有 2 道菜缺嘌呤数据未计入") {
		t.Errorf("missing note = %q", got)
	}
	under := PurineNote(80, 0, true)
	if !strings.Contains(under, "急性期上限 150 mg") || !strings.Contains(under, "还可约 70 mg") {
		t.Errorf("acute under cap = %q", under)
	}
	over := PurineNote(200, 0, true)
	if !strings.Contains(over, "已超急性期上限") {
		t.Errorf("acute over cap = %q", over)
	}
}
