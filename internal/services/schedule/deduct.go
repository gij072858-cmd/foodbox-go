package schedule

import (
	"sort"
	"strings"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
)

// PlanDeduction computes the pantry writes a settlement makes for a set of
// dishes. The preview sheet and the settlement transaction both call it, so
// what the cook confirms is exactly what the transaction writes.
//
// Rules (M4 acceptance):
//   - only required, non-seasoning ingredients consume stock;
//   - a name is matched to pantry rows through the alias table; a depleted row
//     is not stock;
//   - a precise row loses the recipe's amount (summed across the queue) and is
//     marked depleted at zero;
//   - a coarse row steps down one level: 充足 → 还有一些 → 快没了 → 用完;
//   - rows that cannot be consumed keep their stock and are reported as
//     skipped with a one-line reason.
func PlanDeduction(recipes []domain.Recipe, items []domain.Item, m *cookmatch.Matcher) []domain.StockChange {
	idx := newItemIndex(items, m)

	type acc struct {
		item    domain.Item
		amount  float64
		hasAmt  bool
		recipes []string
	}
	order := make([]int64, 0, 8)
	byID := make(map[int64]*acc, 8)
	var skipped []domain.StockChange
	skipSeen := make(map[string]bool, 4)

	addSkipped := func(name, reason string) {
		key := name + "|" + reason
		if skipSeen[key] {
			return
		}
		skipSeen[key] = true
		skipped = append(skipped, domain.StockChange{
			Name: strings.TrimSpace(name), Skipped: true, Reason: reason,
		})
	}
	get := func(it domain.Item) *acc {
		a, ok := byID[it.ID]
		if !ok {
			a = &acc{item: it}
			byID[it.ID] = a
			order = append(order, it.ID)
		}
		return a
	}

	for _, rec := range recipes {
		for _, in := range rec.Ingredients {
			if !in.Required || in.Seasoning {
				continue
			}
			stock := idx.stock(in.Name)
			if len(stock) == 0 {
				addSkipped(in.Name, "不在仓库")
				continue
			}
			it := stock[0]
			if it.QuantityMode != domain.QuantityPrecise || it.Quantity == nil {
				a := get(it)
				a.recipes = appendName(a.recipes, rec.Name)
				continue
			}
			if in.Amount <= 0 {
				addSkipped(in.Name, "没写用量")
				continue
			}
			if !unitsMatch(it.Unit, in.Unit) {
				addSkipped(in.Name, "单位不同")
				continue
			}
			a := get(it)
			a.recipes = appendName(a.recipes, rec.Name)
			a.amount += in.Amount
			a.hasAmt = true
		}
	}

	sort.SliceStable(order, func(i, j int) bool { return order[i] < order[j] })
	changes := make([]domain.StockChange, 0, len(order)+len(skipped))
	for _, id := range order {
		a := byID[id]
		it := a.item
		ch := domain.StockChange{
			ItemID: it.ID, Name: it.Name, Unit: it.Unit, Recipes: a.recipes,
		}
		precise := it.QuantityMode == domain.QuantityPrecise && it.Quantity != nil
		if precise {
			ch.Precise = true
			ch.Before = *it.Quantity
			ch.After = *it.Quantity
			if a.hasAmt {
				left := *it.Quantity - a.amount
				if left <= 0.0001 {
					left, ch.Depleted = 0, true
				}
				ch.After = left
			} else {
				ch.Skipped, ch.Reason = true, "没写用量"
			}
		} else {
			ch.Precise = false
			ch.BeforeLevel = it.GranularLevel
			if ch.BeforeLevel == "" {
				ch.BeforeLevel = domain.GranularSome
			}
			switch ch.BeforeLevel {
			case domain.GranularPlenty:
				ch.AfterLevel = domain.GranularSome
			case domain.GranularSome:
				ch.AfterLevel = domain.GranularLow
			case domain.GranularLow:
				ch.AfterLevel = ""
				ch.Depleted = true
			default:
				ch.BeforeLevel = domain.GranularSome
				ch.AfterLevel = domain.GranularLow
			}
		}
		changes = append(changes, ch)
	}
	return append(changes, skipped...)
}

// unitsMatch reports whether a pantry row's unit can be compared with the
// recipe's amount. An empty unit on either side is taken as the same unit.
func unitsMatch(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return true
	}
	return strings.EqualFold(a, b)
}
