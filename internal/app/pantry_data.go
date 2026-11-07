package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
)

// setItems publishes a new pantry snapshot and rebuilds the derived views.
// It runs on the main thread.
func (a *App) setItems(items []domain.Item) {
	a.items = items
	a.itemCount = len(items)
	byID := make(map[int64]domain.Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	a.itemsByID = byID
	a.recomputePantryView()
	a.recomputeCatalogView()
	a.computeRecipeMatches()
	a.computeShopping()
}

func (a *App) recomputePantryView() {
	now := time.Now().Unix()
	q := strings.ToLower(strings.TrimSpace(a.pantryQuery))
	out := make([]domain.Item, 0, len(a.items))
	for _, it := range a.items {
		switch a.pantryFilter {
		case "临期":
			if !it.NeedsEatingFirst(now) {
				continue
			}
		case "在用":
			if it.IsDepleted {
				continue
			}
		case "用完":
			if !it.IsDepleted {
				continue
			}
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(it.Name), q) &&
			!strings.Contains(strings.ToLower(it.Category), q) {
			continue
		}
		out = append(out, it)
	}
	a.pantryView = out
}

// computeRecipeMatches derives the three availability states for all 944
// recipes after a pantry change. It is one stock-set build and a few set
// lookups per ingredient, kept off the frame.
func (a *App) computeRecipeMatches() {
	if a.matcher == nil {
		return
	}
	if a.pantryOff {
		// No pantry means no availability state to show; clear the cache so
		// reopening the pantry recomputes instead of showing stale counts.
		a.recipeMatches = map[int64]cookmatch.Result{}
		a.canCook, a.almost, a.farAway = 0, 0, 0
		return
	}
	a.recipeMatches = a.matcher.MatchAll(a.recipes, a.items)
	a.canCook, a.almost, a.farAway = 0, 0, 0
	for _, res := range a.recipeMatches {
		switch res.Status {
		case cookmatch.CanCook:
			a.canCook++
		case cookmatch.Almost:
			a.almost++
		default:
			a.farAway++
		}
	}
	a.recomputeRecipeView()
}

// addMissingToShopping puts a recipe's missing required ingredients on the
// shopping list, marking that they came from this dish.
func (a *App) addMissingToShopping(rec domain.Recipe) {
	res := a.recipeMatches[rec.ID]
	existing := make(map[string]bool, len(a.shopExtras))
	for _, l := range a.shopExtras {
		existing[l.Name] = true
	}
	added := 0
	for _, in := range res.Missing {
		if existing[in.Name] {
			continue
		}
		a.shopExtras = append(a.shopExtras, domain.ShoppingLine{
			Name: in.Name, Needed: in.Amount, Unit: in.Unit, Recipe: rec.Name,
		})
		existing[in.Name] = true
		added++
	}
	if added == 0 {
		a.notice = "缺失食材已经在购物清单里"
	} else {
		a.notice = fmt.Sprintf("已把 %d 样缺失食材加入购物清单", added)
	}
	a.computeShopping()
}

func (a *App) recomputeCatalogView() {
	q := strings.ToLower(strings.TrimSpace(a.catalogQuery))
	if q == "" {
		a.catalogView = a.catalog
		return
	}
	out := make([]domain.FoodItem, 0, 128)
	for _, it := range a.catalog {
		if strings.Contains(strings.ToLower(it.Name), q) ||
			strings.Contains(strings.ToLower(it.Category), q) {
			out = append(out, it)
		}
	}
	a.catalogView = out
}

// refreshItems reloads the pantry after a write, on a goroutine.
func (a *App) refreshItems() {
	store := a.store
	if store == nil {
		return
	}
	go func() {
		items, err := store.ListItems(context.Background())
		a.post(func() {
			if err != nil {
				a.notice = "读取仓库失败：" + err.Error()
				return
			}
			a.setItems(items)
		})
	}()
}

func (a *App) setNotice(msg string) { a.notice = msg }

// addCatalogItem adds one system-catalog entry to the pantry. An existing
// depleted row is restored instead of duplicated.
func (a *App) addCatalogItem(fi domain.FoodItem) {
	if a.store == nil {
		a.notice = "数据库尚未就绪"
		return
	}
	for _, it := range a.items {
		if it.Name != fi.Name {
			continue
		}
		if it.IsDepleted {
			id := it.ID
			a.setItemDepleted(id, false)
			a.notice = fi.Name + " 已恢复到仓库"
		} else {
			a.notice = fi.Name + " 已经在仓库里"
		}
		return
	}
	it := itemFromCatalog(fi)
	store := a.store
	go func() {
		_, err := store.CreateItem(context.Background(), it)
		a.post(func() {
			if err != nil {
				a.notice = "添加失败：" + err.Error()
				return
			}
			a.notice = "已添加 " + fi.Name
			a.refreshItems()
		})
	}()
}

func itemFromCatalog(fi domain.FoodItem) domain.Item {
	now := time.Now()
	shelf := 0
	if fi.ShelfLifeDays != nil {
		shelf = *fi.ShelfLifeDays
	}
	var expire *time.Time
	if shelf > 0 {
		t := now.AddDate(0, 0, shelf)
		expire = &t
	}
	location := domain.StorageLocation(fi.StorageLocation)
	if location == "" {
		location = domain.StorageFridge
	}
	kind := domain.ItemKind(fi.Kind)
	if kind == "" {
		kind = domain.ItemKindIngredient
	}
	it := domain.Item{
		Name:            fi.Name,
		Category:        fi.Category,
		Kind:            kind,
		QuantityMode:    domain.QuantityGranular,
		GranularLevel:   domain.GranularSome,
		StoredAt:        &now,
		ShelfLifeDays:   fi.ShelfLifeDays,
		ExpireAt:        expire,
		StorageLocation: location,
		IntroWhat:       fi.IntroWhat,
		IntroHow:        fi.IntroHow,
		IntroStore:      fi.IntroStore,
		IntroTip:        fi.IntroTip,
		IsCustom:        true,
	}
	if shelf > 0 {
		it.ShelfLifeDays = &shelf
	}
	return it
}

// addCustomItem reads the custom editor's fields and inserts the item.
func (a *App) addCustomItem() {
	name := strings.TrimSpace(a.customName)
	if name == "" || a.store == nil {
		a.notice = "请先填写名称"
		return
	}
	category := strings.TrimSpace(a.customCat)
	if category == "" {
		category = "未分类"
	}
	now := time.Now()
	it := domain.Item{
		Name:            name,
		Category:        category,
		Kind:            domain.ItemKindIngredient,
		QuantityMode:    domain.QuantityPrecise,
		StoredAt:        &now,
		StorageLocation: domain.StorageFridge,
		IsCustom:        true,
	}
	if q, err := strconv.ParseFloat(strings.TrimSpace(a.customQty), 64); err == nil && q > 0 {
		it.Quantity = &q
		it.Unit = strings.TrimSpace(a.customUnit)
	} else {
		it.QuantityMode = domain.QuantityGranular
		it.GranularLevel = domain.GranularSome
	}
	if days, err := strconv.Atoi(strings.TrimSpace(a.customShelf)); err == nil && days > 0 {
		it.ShelfLifeDays = &days
		t := now.AddDate(0, 0, days)
		it.ExpireAt = &t
	}
	store := a.store
	go func() {
		_, err := store.CreateItem(context.Background(), it)
		a.post(func() {
			if err != nil {
				a.notice = "添加失败：" + err.Error()
				return
			}
			a.customOpen = false
			a.notice = "已添加 " + name
			a.refreshItems()
		})
	}()
}

// quickEdit opens the inline editor with the item's current values.
func (a *App) quickEdit(it domain.Item) {
	a.editID = it.ID
	a.editQty = 0
	if it.Quantity != nil {
		a.editQty = *it.Quantity
	}
	a.editQtyText = strconv.FormatFloat(a.editQty, 'f', 1, 64)
	a.editUnit = it.Unit
	a.editMode = 0
	if it.QuantityMode == domain.QuantityGranular {
		a.editMode = 1
	}
	a.editGranular = 1
	switch it.GranularLevel {
	case domain.GranularPlenty:
		a.editGranular = 0
	case domain.GranularLow:
		a.editGranular = 2
	}
	a.editShelf = 0
	if it.ShelfLifeDays != nil {
		a.editShelf = *it.ShelfLifeDays
	}
	a.editOpen = true
}

// saveQuickEdit writes the editor's values and closes it.
func (a *App) saveQuickEdit() {
	it, ok := a.itemsByID[a.editID]
	if !ok || a.store == nil {
		a.editOpen = false
		return
	}
	if a.editMode == 0 {
		q, err := strconv.ParseFloat(strings.TrimSpace(a.editQtyText), 64)
		if err != nil {
			a.notice = "数量格式不对，请写一个数字"
			return
		}
		it.QuantityMode = domain.QuantityPrecise
		it.Quantity = &q
		it.Unit = strings.TrimSpace(a.editUnit)
	} else {
		it.QuantityMode = domain.QuantityGranular
		it.Quantity = nil
		it.GranularLevel = []domain.GranularLevel{
			domain.GranularPlenty, domain.GranularSome, domain.GranularLow,
		}[a.editGranular]
	}
	if a.editShelf > 0 {
		it.ShelfLifeDays = &a.editShelf
	}
	store := a.store
	go func() {
		err := store.UpdateItem(context.Background(), it)
		a.post(func() {
			if err != nil {
				a.notice = "保存失败：" + err.Error()
				return
			}
			a.editOpen = false
			a.notice = it.Name + " 已更新"
			a.refreshItems()
		})
	}()
}

func (a *App) setItemDepleted(id int64, depleted bool) {
	store := a.store
	if store == nil {
		return
	}
	go func() {
		err := store.SetItemDepleted(context.Background(), id, depleted)
		a.post(func() {
			if err != nil {
				a.notice = "更新失败：" + err.Error()
				return
			}
			a.refreshItems()
		})
	}()
}

func (a *App) deleteItem(id int64) {
	store := a.store
	if store == nil {
		return
	}
	go func() {
		err := store.DeleteItem(context.Background(), id)
		a.post(func() {
			if err != nil {
				a.notice = "删除失败：" + err.Error()
				return
			}
			a.itemDetail = 0
			a.notice = "已移出仓库"
			a.refreshItems()
		})
	}()
}

// addPack adds a starter pack's core (and optionally optional) entries that
// are not already owned, in one transaction.
func (a *App) addPack(pack domain.StarterPack) {
	if a.store == nil {
		return
	}
	owned := make(map[string]bool, len(a.items))
	for _, it := range a.items {
		owned[it.Name] = true
	}
	names := append([]string{}, pack.Core...)
	if a.packOptional {
		for _, name := range pack.Optional {
			if a.packPicks[name] {
				names = append(names, name)
			}
		}
	}
	items := make([]domain.Item, 0, len(names))
	for _, name := range names {
		if owned[name] {
			continue
		}
		fi, ok := a.catalogIndex[name]
		if !ok {
			continue
		}
		items = append(items, itemFromCatalog(fi))
		owned[name] = true
	}
	if len(items) == 0 {
		a.notice = "这个模板的条目都已在仓库"
		return
	}
	store := a.store
	go func() {
		err := store.CreateItems(context.Background(), items)
		a.post(func() {
			if err != nil {
				a.notice = "导入失败：" + err.Error()
				return
			}
			a.notice = fmt.Sprintf("已从「%s」加入 %d 样", pack.Region, len(items))
			a.refreshItems()
		})
	}()
}

// computeShopping derives the shopping list: depleted pantry rows, plus
// required ingredients of the dishes added to the schedule that are not in
// the pantry. It runs when items or the schedule change.
//
// It is also the single place that refreshes M4's derived views (prep list and
// timeline): they depend on both the queue and the pantry snapshot, and every
// path that changes either one ends up here.
func (a *App) computeShopping() {
	if a.pantryOff {
		a.shopping = nil
		a.shopExtras = nil
		a.recomputeScheduleDerived()
		return
	}
	lines := make([]domain.ShoppingLine, 0, 32)
	seen := make(map[string]int)
	inPantry := make(map[string]bool)
	for _, it := range a.items {
		if it.IsDepleted {
			lines = append(lines, domain.ShoppingLine{
				Name: it.Name, InPantry: true, Depleted: true, FromPantry: true,
			})
			continue
		}
		inPantry[it.Name] = true
	}
	for _, line := range a.shopExtras {
		if inPantry[line.Name] {
			continue
		}
		if _, ok := seen[line.Name]; ok {
			continue
		}
		seen[line.Name] = len(lines)
		lines = append(lines, line)
	}
	for _, item := range a.schedule {
		rec, ok := a.recipeByID[item.RecipeID]
		if !ok {
			continue
		}
		for _, in := range rec.Ingredients {
			if !in.Required || inSeasoning(in) {
				continue
			}
			if inPantry[in.Name] {
				continue
			}
			if i, ok := seen[in.Name]; ok {
				lines[i].Needed += in.Amount
				continue
			}
			seen[in.Name] = len(lines)
			lines = append(lines, domain.ShoppingLine{
				Name: in.Name, Needed: in.Amount, Unit: in.Unit, Recipe: rec.Name,
			})
		}
	}
	a.shopping = lines
	a.recomputeScheduleDerived()
}

func inSeasoning(in domain.Ingredient) bool { return in.Seasoning }
