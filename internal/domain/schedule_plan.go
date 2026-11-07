package domain

// Resource keys as the seed data writes them in steps.resources. A step may
// name several; the planner places it on the busiest one (stove before board
// before hand) and keeps the rest for the scheduler's capacity checks.
const (
	ResourceHand    = "hand"
	ResourceStove   = "stove"
	ResourceBoard   = "board"
	ResourceWaiting = "waiting"
)

// Timeline is the parallel plan for a queue of dishes: one lane per resource
// slot, plus the totals the page shows.
//
// Minutes are whole minutes from the start of the run. All steps are at least
// one minute long, so a lane never contains overlapping blocks.
type Timeline struct {
	Lanes []TimelineLane
	// Steps is the whole plan in execution order: by start minute, then by
	// the dish's queue position, then by its own step index. The lyric flow
	// walks this, so parallel dishes interleave instead of finishing one
	// dish at a time.
	Steps         []TimelineStep
	TotalMinutes  int
	SerialMinutes int
	SavedMinutes  int
	StoveCount    int
}

// Empty reports whether the queue had no usable steps.
func (t Timeline) Empty() bool { return len(t.Lanes) == 0 || t.TotalMinutes == 0 }

// TimelineLane is one resource slot's occupied stretch of the run: a stove,
// a board, a hands-on track, or the passive-wait track.
type TimelineLane struct {
	// Resource is the lane's resource key (hand/stove/board/waiting).
	Resource string
	// Slot is the 1-based index within the resource. Hand and waiting tracks
	// carry their own 1-based index among those tracks.
	Slot int
	// Label is the lane's Chinese name, such as "灶台 2".
	Label string
	// Start and End are the lane's first and last minutes.
	Start int
	End   int
	// Steps are the lane's steps, in run order.
	Steps []TimelineStep
}

// TimelineStep is one step placed on a lane.
type TimelineStep struct {
	RecipeID   int64
	RecipeName string
	// Order is the dish's position in the input queue; ties at the same
	// start minute keep the queue's order.
	Order       int
	StepIndex   int
	Servings    int
	Instruction string
	Start       int
	End         int
	Resource    string
	Resources   []string
	// Waiting marks a step the cook can walk away from (braising, steaming,
	// marinating). It is what the run page offers a countdown for.
	Waiting bool
}

// Minutes is the step's length.
func (s TimelineStep) Minutes() int { return s.End - s.Start }

// RunLine is one lyric line of a cooking run: a step of one queued dish, in
// execution order.
type RunLine struct {
	RecipeID   int64
	RecipeName string
	StepIndex  int
	Text       string
	Minutes    int
	// Waiting marks a passive-wait step; the run page can hang a countdown
	// for it above the lyric flow.
	Waiting bool
}

// PrepLine is one aggregated ingredient across the queued dishes.
type PrepLine struct {
	// Name is the ingredient as the first recipe spells it.
	Name string
	// Needed is the summed amount; 0 means "适量" (no amount written).
	Needed float64
	Unit   string
	// Recipes are the dishes that need it, in queue order.
	Recipes []string
	// InStock reports a non-depleted pantry row was matched through aliases.
	InStock bool
	// Have is the summed precise quantity in stock; 0 for coarse rows.
	Have float64
	// Missing is what still has to be bought. A coarse (颗颗粒度) row in
	// stock counts as covered, so it is 0.
	Missing float64
	// Group is "fresh" (生鲜), "staple" (底料) or "seasoning" (调料).
	Group string
	// Seasoning lines are listed for readiness but never deducted.
	Seasoning bool
	// RawMeat marks meat, poultry and fish so the prep page can warn about
	// raw/cooked separation.
	RawMeat bool
}

// PrepGroups are the group keys of PrepLine.Group, in display order.
var PrepGroups = []string{"fresh", "staple", "seasoning"}

// PrepGroupTitle is the Chinese heading of a prep group.
func PrepGroupTitle(group string) string {
	switch group {
	case "fresh":
		return "生鲜"
	case "staple":
		return "底料"
	case "seasoning":
		return "调料"
	}
	return ""
}

// PrepGroup is one titled block of prep lines.
type PrepGroup struct {
	Group string
	Title string
	Lines []PrepLine
}

// StockChange is one pantry write a settlement makes: the deduction the cook
// sees in the confirmation sheet, and the row update the transaction runs.
type StockChange struct {
	ItemID int64
	Name   string
	Unit   string
	// Precise rows carry a quantity; coarse rows carry a level instead.
	Precise bool
	Before  float64
	After   float64
	// BeforeLevel and AfterLevel are the coarse stock levels.
	BeforeLevel GranularLevel
	AfterLevel  GranularLevel
	// Depleted marks a row the settlement uses up.
	Depleted bool
	// Recipes are the dishes that consumed it.
	Recipes []string
	// Skipped changes were matched but not deducted; Reason says why.
	Skipped bool
	Reason  string
}

// SettlePreview is what the confirmation sheet shows before a settlement
// commits: the dishes, the XP, and the pantry writes.
type SettlePreview struct {
	RecipeIDs []int64
	Dishes    int
	XP        int
	Changes   []StockChange
}

// Deducted counts the pantry rows the settlement will actually change.
func (p SettlePreview) Deducted() int {
	n := 0
	for _, ch := range p.Changes {
		if !ch.Skipped {
			n++
		}
	}
	return n
}

// Skipped counts the matched rows the settlement leaves alone.
func (p SettlePreview) Skipped() int { return len(p.Changes) - p.Deducted() }

// GranularText is the stock-level wording used on cards and in the sheet.
func GranularText(level GranularLevel) string {
	switch level {
	case GranularPlenty:
		return "充足"
	case GranularSome:
		return "还有一些"
	case GranularLow:
		return "快没了"
	case "":
		return "已用完"
	}
	return string(level)
}

// AmountText is a quantity with its unit, or just the number when the unit is
// unknown.
func AmountText(v float64, unit string) string {
	if unit == "" {
		return TrimFloat(v)
	}
	return TrimFloat(v) + " " + unit
}

// Text is the one-line wording of a stock change.
func (c StockChange) Text() string {
	if c.Skipped {
		return c.Name + " " + c.Reason
	}
	if c.Precise {
		before := AmountText(c.Before, c.Unit)
		after := AmountText(c.After, c.Unit)
		if c.Depleted {
			after = "用完"
		}
		return c.Name + " " + before + " → " + after
	}
	if c.Depleted {
		return c.Name + " " + GranularText(c.BeforeLevel) + " → 用完"
	}
	return c.Name + " " + GranularText(c.BeforeLevel) + " → " + GranularText(c.AfterLevel)
}
