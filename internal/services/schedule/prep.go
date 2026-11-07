package schedule

import (
	"sort"
	"strings"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
)

// itemIndex answers "is this ingredient in the pantry?" through the shipped
// alias table, the same normalization the three-state matcher uses. Depleted
// rows are left out: they are already marked used up.
type itemIndex struct {
	byCanon map[string][]domain.Item
	m       *cookmatch.Matcher
}

func newItemIndex(items []domain.Item, m *cookmatch.Matcher) *itemIndex {
	idx := &itemIndex{byCanon: make(map[string][]domain.Item, len(items)), m: m}
	for _, it := range items {
		if it.IsDepleted {
			continue
		}
		key := idx.canonical(it.Name)
		idx.byCanon[key] = append(idx.byCanon[key], it)
	}
	for key := range idx.byCanon {
		list := idx.byCanon[key]
		sort.SliceStable(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		idx.byCanon[key] = list
	}
	return idx
}

func (i *itemIndex) canonical(name string) string {
	if i.m != nil {
		return i.m.Canonical(name)
	}
	return cookmatch.Normalize(name)
}

// stock returns the non-depleted rows matching an ingredient name.
func (i *itemIndex) stock(name string) []domain.Item {
	return i.byCanon[i.canonical(name)]
}

// BuildPrep aggregates the queued dishes' required ingredients into one list.
//
// Rows merge through the alias table, so 番茄 and 西红柿 become one line. A
// coarse (granular) pantry row counts as covered; only precise quantities are
// compared, and the shortfall is what still has to be bought. Required
// seasonings are listed for readiness — group "seasoning" — but never
// deducted.
func BuildPrep(recipes []domain.Recipe, items []domain.Item, m *cookmatch.Matcher) []domain.PrepLine {
	idx := newItemIndex(items, m)
	order := make([]string, 0, 16)
	byKey := make(map[string]*domain.PrepLine, 16)

	for _, rec := range recipes {
		for _, in := range rec.Ingredients {
			if !in.Required {
				continue
			}
			key := idx.canonical(in.Name)
			line, ok := byKey[key]
			if !ok {
				line = &domain.PrepLine{Name: strings.TrimSpace(in.Name), Unit: in.Unit, Seasoning: in.Seasoning}
				byKey[key] = line
				order = append(order, key)
			}
			if line.Unit == "" && in.Unit != "" {
				line.Unit = in.Unit
			}
			if in.Amount > 0 {
				line.Needed += in.Amount
			}
			line.Recipes = appendName(line.Recipes, rec.Name)
		}
	}

	for _, key := range order {
		line := byKey[key]
		stock := idx.stock(line.Name)
		line.InStock = len(stock) > 0
		precise := 0.0
		hasPrecise := false
		for _, it := range stock {
			if it.QuantityMode != domain.QuantityPrecise || it.Quantity == nil {
				continue
			}
			if line.Unit != "" && it.Unit != "" && !strings.EqualFold(strings.TrimSpace(line.Unit), strings.TrimSpace(it.Unit)) {
				continue
			}
			precise += *it.Quantity
			hasPrecise = true
		}
		line.Have = precise
		switch {
		case !line.InStock:
			line.Missing = line.Needed
		case hasPrecise && line.Needed > precise:
			line.Missing = line.Needed - precise
		}
		line.Group = prepGroup(line, stock)
		line.RawMeat = rawMeatName(line.Name)
	}

	lines := make([]domain.PrepLine, 0, len(order))
	for _, key := range order {
		lines = append(lines, *byKey[key])
	}
	sort.SliceStable(lines, func(i, j int) bool {
		gi, gj := groupRank(lines[i].Group), groupRank(lines[j].Group)
		if gi != gj {
			return gi < gj
		}
		if lines[i].Name != lines[j].Name {
			return lines[i].Name < lines[j].Name
		}
		return lines[i].Unit < lines[j].Unit
	})
	return lines
}

// GroupPrep splits the prep lines into their titled display groups.
func GroupPrep(lines []domain.PrepLine) []domain.PrepGroup {
	groups := make([]domain.PrepGroup, 0, len(domain.PrepGroups))
	for _, group := range domain.PrepGroups {
		g := domain.PrepGroup{Group: group, Title: domain.PrepGroupTitle(group)}
		for _, line := range lines {
			if line.Group == group {
				g.Lines = append(g.Lines, line)
			}
		}
		if len(g.Lines) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}

// prepGroup decides which display block a line belongs to: seasonings first
// (they are never deducted), then the shelf staples, then fresh food.
func prepGroup(line *domain.PrepLine, stock []domain.Item) string {
	if line.Seasoning {
		return "seasoning"
	}
	if len(stock) > 0 {
		if domain.ZoneOf(stock[0]) == domain.ZoneStaple {
			return "staple"
		}
		return "fresh"
	}
	if domain.StapleNames[line.Name] {
		return "staple"
	}
	return "fresh"
}

func groupRank(group string) int {
	for i, g := range domain.PrepGroups {
		if g == group {
			return i
		}
	}
	return len(domain.PrepGroups)
}

// rawMeatName is the light safety hint the prep page shows: the cook should
// cut vegetables before raw meat and wash the board afterwards.
func rawMeatName(name string) bool {
	if strings.Contains(name, "蛋") {
		return false
	}
	for _, key := range []string{
		"肉", "鱼", "虾", "蟹", "贝", "蛤", "鱿", "排骨", "鸡胸", "鸡腿",
		"鸡翅", "鸭", "鹅", "猪", "羊", "培根", "火腿", "香肠", "牛腩", "里脊",
	} {
		if strings.Contains(name, key) {
			return true
		}
	}
	return false
}

// appendName adds a dish name once, keeping queue order.
func appendName(names []string, name string) []string {
	for _, n := range names {
		if n == name {
			return names
		}
	}
	return append(names, name)
}
