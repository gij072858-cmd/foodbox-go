package domain

import "testing"

func TestVersionGroupOf(t *testing.T) {
	cases := []struct {
		name   string
		recipe Recipe
		want   string
	}{
		{"explicit group wins", Recipe{Name: "口水鸡（家常版）", VersionGroup: "口水鸡"}, "口水鸡"},
		{"explicit group without note", Recipe{Name: "宫保鸡丁", VersionGroup: "宫保鸡丁"}, "宫保鸡丁"},
		{"name note is the fallback", Recipe{Name: "口水鸡（整鸡版）"}, "口水鸡"},
		{"half-width note", Recipe{Name: "口水鸡(整鸡版)"}, "口水鸡"},
		{"plain name is its own group", Recipe{Name: "番茄炒蛋"}, "番茄炒蛋"},
		{"a note inside the name stays", Recipe{Name: "关东煮福袋（油豆腐包年糕）"}, "关东煮福袋"},
		{"note must end the name", Recipe{Name: "（前注）红烧肉"}, "（前注）红烧肉"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VersionGroupOf(c.recipe); got != c.want {
				t.Fatalf("VersionGroupOf(%q) = %q, want %q", c.recipe.Name, got, c.want)
			}
		})
	}
}

func TestVariantLabelOf(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"口水鸡（整鸡版）", "整鸡版"},
		{"口水鸡(家常版)", "家常版"},
		{"宫保鸡丁", ""},
	}
	for _, c := range cases {
		if got := VariantLabelOf(Recipe{Name: c.name}); got != c.want {
			t.Errorf("VariantLabelOf(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
