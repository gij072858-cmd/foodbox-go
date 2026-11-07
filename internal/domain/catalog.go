package domain

// FoodItem is an entry of the read-only system catalog. It is not a pantry
// item: the user owns an item only after adding it from this catalog.
type FoodItem struct {
	Name            string `json:"name"`
	Category        string `json:"category"`
	Kind            string `json:"kind"`
	ShelfLifeDays   *int   `json:"shelfLifeDays"`
	StorageLocation string `json:"storageLocation"`
	IntroWhat       string `json:"introWhat"`
	IntroHow        string `json:"introHow"`
	IntroStore      string `json:"introStore"`
	IntroTip        string `json:"introTip"`
	Image           string `json:"-"`
}

// CatalogGroup is a category of the system catalog.
type CatalogGroup struct {
	Name  string
	Items []FoodItem
}
