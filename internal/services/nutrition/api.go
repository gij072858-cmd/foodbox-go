// Package nutrition estimates a dish's energy and macronutrients from the
// embedded food table, and turns a body profile into a daily target.
//
// The numbers are estimates for a home cook, never medical advice: the
// estimator reports how much of a dish it actually covered, and the UI says
// 估算 next to every number.
package nutrition

// Nutrients is one dish's estimate per base serving.
type Nutrients struct {
	Kcal     float64 `json:"kcal"`
	ProteinG float64 `json:"proteinG"`
	FatG     float64 `json:"fatG"`
	CarbG    float64 `json:"carbG"`
	FiberG   float64 `json:"fiberG"`
	SodiumMg float64 `json:"sodiumMg"`
	PurineMg float64 `json:"purineMg"`
	// Coverage is "full", "partial" or "none": how many ingredients the
	// table actually covered.
	Coverage string `json:"coverage"`
}

// Add sums two estimates. Coverage degrades to the weaker one.
func (n Nutrients) Add(o Nutrients) Nutrients {
	return Nutrients{
		Kcal:     n.Kcal + o.Kcal,
		ProteinG: n.ProteinG + o.ProteinG,
		FatG:     n.FatG + o.FatG,
		CarbG:    n.CarbG + o.CarbG,
		FiberG:   n.FiberG + o.FiberG,
		SodiumMg: n.SodiumMg + o.SodiumMg,
		PurineMg: n.PurineMg + o.PurineMg,
		Coverage: weakerCoverage(n.Coverage, o.Coverage),
	}
}

// Scale multiplies an estimate.
func (n Nutrients) Scale(k float64) Nutrients {
	n.Kcal *= k
	n.ProteinG *= k
	n.FatG *= k
	n.CarbG *= k
	n.FiberG *= k
	n.SodiumMg *= k
	n.PurineMg *= k
	return n
}

// HasData reports whether the estimate carries any energy at all.
func (n Nutrients) HasData() bool { return n.Kcal > 0 || n.ProteinG > 0 }

func weakerCoverage(a, b string) string {
	rank := map[string]int{CoverageNone: 0, CoveragePartial: 1, CoverageFull: 2}
	if rank[b] < rank[a] {
		return b
	}
	return a
}

// Entry is one row of food_nutrition.json, per 100 g edible portion.
type Entry struct {
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	EnergyKcal   *float64 `json:"energyKcal"`
	ProteinG     *float64 `json:"proteinG"`
	FatG         *float64 `json:"fatG"`
	CarbG        *float64 `json:"carbG"`
	FiberG       *float64 `json:"fiberG"`
	SodiumMg     *float64 `json:"sodiumMg"`
	PurineMg     *float64 `json:"purineMg"`
	PurineLevel  string   `json:"purineLevel"`
	GI           *float64 `json:"gi"`
	GILevel      string   `json:"giLevel"`
	EdibleFactor *float64 `json:"edibleFactor"`
	Coverage     string   `json:"coverage"`
}

// Catalog is the embedded nutrition table, keyed by normalized name.
type Catalog struct {
	entries map[string]Entry
	// aliases maps a normalized alias (and each canonical name to itself) to
	// the normalized canonical name.
	aliases map[string]string
	// purines is the appendix table: name -> mg per 100 g.
	purines map[string]float64
	// purineLow and purineMedium are the appendix thresholds in mg/100 g:
	// below low is 低, below medium is 中, the rest is 高.
	purineLow    float64
	purineMedium float64
}

// NewCatalog loads the embedded table.
func NewCatalog() (*Catalog, error) {
	c := &Catalog{
		entries: make(map[string]Entry),
		aliases: make(map[string]string),
		purines: make(map[string]float64),
	}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

// EnergyInput is the body profile the daily target is computed from.
type EnergyInput struct {
	HeightCm      int
	WeightKg      int
	Age           int
	Sex           string // "male" / "female" / "" (unknown)
	ActivityLevel string // "sedentary" / "light" / "moderate" / "active" / "veryActive"
	LifeStage     string // "pregnant" / "lactating" / "postpartum" / "" (none)
	Goals         []string
}

// Energy is a daily energy and macronutrient target.
type Energy struct {
	BMR        int
	TDEE       int
	TargetKcal int
	ProteinG   int
	FatG       int
	CarbG      int
	// Note says which inputs were missing, in one short sentence.
	Note string
}

// EnergyFor computes the daily target from a body profile.
func EnergyFor(in EnergyInput) Energy { return computeEnergy(in) }
