package domain

import "time"

// ItemKind separates ingredients from seasonings.
type ItemKind string

const (
	ItemKindIngredient ItemKind = "ingredient"
	ItemKindSeasoning  ItemKind = "seasoning"
)

// StorageLocation is where an item is kept.
type StorageLocation string

const (
	StorageFridge   StorageLocation = "fridge"
	StorageFreezer  StorageLocation = "freezer"
	StorageRoomTemp StorageLocation = "roomTemp"
)

// QuantityMode says how the user tracks an item: exact amounts or a coarse
// level ("enough / some / low").
type QuantityMode string

const (
	QuantityPrecise  QuantityMode = "precise"
	QuantityGranular QuantityMode = "granular"
)

// GranularLevel is a coarse stock level for items the user does not count.
type GranularLevel string

const (
	GranularPlenty GranularLevel = "plenty"
	GranularSome   GranularLevel = "some"
	GranularLow    GranularLevel = "low"
)

// Item is one pantry entry.
type Item struct {
	ID              int64
	Name            string
	Category        string
	Kind            ItemKind
	QuantityMode    QuantityMode
	Quantity        *float64
	Unit            string
	GranularLevel   GranularLevel
	StoredAt        *time.Time
	ShelfLifeDays   *int
	ExpireAt        *time.Time
	StorageLocation StorageLocation
	Opened          *bool
	Barcode         string
	ImagePath       string
	IntroWhat       string
	IntroHow        string
	IntroStore      string
	IntroTip        string
	IsCustom        bool
	IsDepleted      bool
	Note            string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
