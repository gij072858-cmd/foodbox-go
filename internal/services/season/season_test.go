package season

import (
	"testing"
	"time"
)

func TestCurrentMonth(t *testing.T) {
	now := time.Date(2026, time.November, 5, 14, 0, 0, 0, time.Local)
	if got := CurrentMonth(now); got != 11 {
		t.Fatalf("CurrentMonth = %d, want 11", got)
	}
	// The month comes from the value's own calendar, not the wall clock.
	other := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	if got := CurrentMonth(other); got != 1 {
		t.Fatalf("CurrentMonth = %d, want 1", got)
	}
}

func TestInSeason(t *testing.T) {
	cases := []struct {
		name   string
		months []int
		month  int
		want   bool
	}{
		{"empty is all year", nil, 7, true},
		{"twelve months is all year", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, 2, true},
		{"inside spring", []int{3, 4, 5}, 4, true},
		{"before spring", []int{3, 4, 5}, 2, false},
		{"after spring", []int{3, 4, 5}, 6, false},
		{"winter crosses the year", []int{12, 1, 2}, 1, true},
		{"winter crosses the year backwards", []int{12, 1, 2}, 12, true},
		{"winter ends with february", []int{12, 1, 2}, 3, false},
		{"unsorted input", []int{9, 10, 11}, 10, true},
		{"out of range ignored", []int{0, 13, 4}, 4, true},
		{"out of range alone", []int{0, 13}, 4, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := InSeason(c.months, c.month); got != c.want {
				t.Fatalf("InSeason(%v, %d) = %v, want %v", c.months, c.month, got, c.want)
			}
		})
	}
}

func TestMarked(t *testing.T) {
	if Marked(nil) {
		t.Error("no months must not be marked")
	}
	all := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	if Marked(all) {
		t.Error("twelve months must not be marked")
	}
	if !Marked([]int{3, 4, 5}) {
		t.Error("spring must be marked")
	}
	if !Marked([]int{12, 1, 2}) {
		t.Error("winter must be marked")
	}
}

func TestLabel(t *testing.T) {
	all := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	cases := []struct {
		months []int
		want   string
	}{
		{nil, "全年"},
		{all, "全年"},
		{[]int{3, 4, 5}, "春 · 3–5 月"},
		{[]int{3}, "春 · 3 月"},
		{[]int{6, 7, 8}, "夏 · 6–8 月"},
		{[]int{9, 10, 11}, "秋 · 9–11 月"},
		{[]int{12, 1, 2}, "冬 · 12–2 月"},
		{[]int{12, 1}, "冬 · 12–1 月"},
		{[]int{1, 2}, "冬 · 1–2 月"},
		{[]int{3, 4, 5, 6, 7, 8}, "春 · 3–5 月、夏 · 6–8 月"},
		{[]int{6, 7, 8, 9, 10, 11}, "夏 · 6–8 月、秋 · 9–11 月"},
		{[]int{11, 12, 1, 2, 3}, "春 · 3 月、秋 · 11 月、冬 · 12–2 月"},
	}
	for _, c := range cases {
		if got := Label(c.months); got != c.want {
			t.Errorf("Label(%v) = %q, want %q", c.months, got, c.want)
		}
	}
}

func TestLabelWinterCrossingYear(t *testing.T) {
	// The winter window starts in December of one year and ends in January
	// of the next; it must stay one run, not two.
	if got := Label([]int{1, 12}); got != "冬 · 12–1 月" {
		t.Fatalf("Label([1 12]) = %q, want %q", got, "冬 · 12–1 月")
	}
	if got := Label([]int{2, 12, 1}); got != "冬 · 12–2 月" {
		t.Fatalf("Label([2 12 1]) = %q, want %q", got, "冬 · 12–2 月")
	}
}

func TestSeasonName(t *testing.T) {
	want := map[int]string{
		1: "冬", 2: "冬", 3: "春", 4: "春", 5: "春", 6: "夏",
		7: "夏", 8: "夏", 9: "秋", 10: "秋", 11: "秋", 12: "冬",
		0: "", 13: "",
	}
	for m, name := range want {
		if got := SeasonName(m); got != name {
			t.Errorf("SeasonName(%d) = %q, want %q", m, got, name)
		}
	}
}
