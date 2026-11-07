package domain

// StarterPack is one selectable regional starter pack from
// starter_packs.json.
type StarterPack struct {
	ID       string   `json:"id"`
	Region   string   `json:"region"`
	Core     []string `json:"core"`
	Optional []string `json:"optional"`
}

// ShoppingLine is one row of the derived shopping list.
type ShoppingLine struct {
	Name       string
	Needed     float64
	Unit       string
	Recipe     string
	InPantry   bool
	Depleted   bool
	FromPantry bool
}

// PantryZone separates the photo rail (fresh food) from the staple shelf.
type PantryZone string

const (
	ZoneFresh  PantryZone = "fresh"
	ZoneStaple PantryZone = "staple"
)

// StapleCategories are ingredient categories that live on the shelf even
// though they are ingredients (dry goods, pickles, cans, drinks, spices).
var StapleCategories = map[string]bool{
	"干货": true, "腌渍": true, "罐头": true, "酒饮": true,
	"干香辛料": true, "复合调味": true,
}

// StapleNames are long-keeping aromatics the Flutter app puts on the shelf.
var StapleNames = map[string]bool{
	"洋葱": true, "生姜": true, "蒜": true, "干葱头": true,
	"嫩姜": true, "红葱头": true,
}

// ZoneOf returns the zone an item belongs to. Seasonings and long-keeping
// ingredients go on the staple shelf; everything else is fresh.
func ZoneOf(it Item) PantryZone {
	if it.Kind == ItemKindSeasoning || StapleCategories[it.Category] || StapleNames[it.Name] {
		return ZoneStaple
	}
	return ZoneFresh
}

// NeedsEatingFirst reports expiry within 3 days, or a "low" granular level.
func (it Item) NeedsEatingFirst(nowSec int64) bool {
	if it.IsDepleted {
		return false
	}
	if it.GranularLevel == GranularLow {
		return true
	}
	if it.ExpireAt == nil {
		return false
	}
	return it.ExpireAt.Unix()-nowSec <= 3*24*3600
}

// RemainingText is the short stock label shown on a pantry card.
func (it Item) RemainingText() string {
	if it.IsDepleted {
		return "已用完"
	}
	if it.QuantityMode == QuantityPrecise && it.Quantity != nil {
		return TrimFloat(*it.Quantity) + " " + it.Unit
	}
	switch it.GranularLevel {
	case GranularPlenty:
		return "充足"
	case GranularLow:
		return "快没了"
	default:
		return "还有一些"
	}
}
