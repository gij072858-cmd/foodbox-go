// Package schedule is M4's planner: it turns a queue of dishes into a
// parallel timeline, aggregates their ingredients into one prep list, and
// computes the pantry writes a settlement makes.
//
// It is pure logic: no UI, no database. The app builds the inputs from its
// loaded snapshot and the sqlite store reuses the deduction plan inside its
// settlement transaction, so the sheet the cook confirms and the rows the
// transaction writes can never disagree.
package schedule

import (
	"sort"
	"strings"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

const (
	// DefaultStoveCount is the stove capacity of a kitchen that never set one.
	DefaultStoveCount = 2
	// MaxStoveCount caps how many burners a lane chart will lay out.
	MaxStoveCount = 4
	// BoardCount is how many cutting boards the planner assumes: the board and
	// the knife are shared, but two boards' worth of work can overlap.
	BoardCount = 2
	// DefaultStepMinutes is the length of a step that has none. Every step is
	// at least one minute, so a lane never holds zero-length blocks.
	DefaultStepMinutes = 1
	// maxClampMinutes keeps a dirty duration from exploding the fit search.
	maxClampMinutes = 24 * 60
)

// StepMinutes is a step's planned length: its own minutes rounded to whole
// minutes and clamped to at least DefaultStepMinutes. Dirty values (missing,
// negative, absurd) become one minute rather than breaking the plan.
func StepMinutes(st domain.Step) int {
	if st.DurationMinutes == nil {
		return DefaultStepMinutes
	}
	m := *st.DurationMinutes
	if m != m || m > maxClampMinutes {
		return maxClampMinutes
	}
	minutes := int(m + 0.5)
	if minutes < DefaultStepMinutes {
		return DefaultStepMinutes
	}
	return minutes
}

// PrimaryResource is the resource a step occupies for its whole length. A
// braise names both stove and waiting; it keeps the burner, so the stove wins.
// A step that names nothing is hands-on work.
func PrimaryResource(resources []string) string {
	for _, want := range []string{domain.ResourceStove, domain.ResourceBoard, domain.ResourceHand, domain.ResourceWaiting} {
		for _, r := range resources {
			if strings.EqualFold(strings.TrimSpace(r), want) {
				return want
			}
		}
	}
	return domain.ResourceHand
}

// IsWaiting reports whether the step is a passive wait the cook can walk away
// from. Such steps are what the run page offers a countdown for.
func IsWaiting(resources []string) bool {
	for _, r := range resources {
		if strings.EqualFold(strings.TrimSpace(r), domain.ResourceWaiting) {
			return true
		}
	}
	return false
}

// capacity returns how many steps of a resource may run at the same moment.
// Zero means the resource does not limit the plan.
func capacity(resource string, stoveCount int) int {
	switch resource {
	case domain.ResourceStove:
		return stoveCount
	case domain.ResourceBoard:
		return BoardCount
	}
	return 0
}

// ClampStoveCount keeps a dirty kitchen setting usable.
func ClampStoveCount(n int) int {
	if n < 1 {
		return DefaultStoveCount
	}
	if n > MaxStoveCount {
		return MaxStoveCount
	}
	return n
}

type dishState struct {
	recipe    domain.Recipe
	steps     []domain.Step
	index     int
	next      int
	ready     int
	remaining int
	servings  int
}

type placedStep struct {
	dish     int
	recipe   domain.Recipe
	step     domain.Step
	index    int
	servings int
	start    int
	end      int
	res      string
}

// BuildTimeline lays the queued dishes' steps onto parallel resource lanes.
//
// Rules (M4 acceptance):
//   - a dish's steps keep their order; the next step cannot start before the
//     previous one ends;
//   - across dishes the plan is greedy: the dish with the most work left goes
//     next, and its step takes the earliest moment its resource has room;
//   - a resource runs at most its capacity at any moment: stove work is capped
//     by the kitchen's burner count (1-4, default 2), a board holds two,
//     hands-on work and passive waits do not block each other;
//   - the input order is the tie-break, so the same input always produces the
//     same timeline.
func BuildTimeline(recipes []domain.Recipe, steps map[int64][]domain.Step, servings map[int64]int, stoveCount int) domain.Timeline {
	stoveCount = ClampStoveCount(stoveCount)
	tl := domain.Timeline{StoveCount: stoveCount}

	states := make([]*dishState, 0, len(recipes))
	for _, rec := range recipes {
		list := append([]domain.Step(nil), steps[rec.ID]...)
		if len(list) == 0 {
			continue
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].OrderIndex < list[j].OrderIndex })
		sv := rec.BaseServings
		if v, ok := servings[rec.ID]; ok && v > 0 {
			sv = v
		}
		st := &dishState{recipe: rec, steps: list, servings: sv, index: len(states)}
		for _, step := range list {
			st.remaining += StepMinutes(step)
		}
		tl.SerialMinutes += st.remaining
		states = append(states, st)
	}
	if len(states) == 0 {
		return tl
	}

	placed := make([]placedStep, 0, 32)
	total := 0
	for _, st := range states {
		total += len(st.steps)
	}
	for round := 0; round < total; round++ {
		st := pickLongestChain(states)
		if st == nil {
			break
		}
		step := st.steps[st.next]
		dur := StepMinutes(step)
		res := PrimaryResource(step.Resources)
		start := earliestFit(placed, res, stoveCount, st.ready, dur)
		p := placedStep{
			dish:     st.index,
			recipe:   st.recipe,
			step:     step,
			index:    st.next,
			servings: st.servings,
			start:    start,
			end:      start + dur,
			res:      res,
		}
		placed = append(placed, p)
		st.next++
		st.ready = p.end
		st.remaining -= dur
		tl.TotalMinutes = maxInt(tl.TotalMinutes, p.end)
	}

	tl.SavedMinutes = tl.SerialMinutes - tl.TotalMinutes
	if tl.SavedMinutes < 0 {
		tl.SavedMinutes = 0
	}
	tl.Lanes = buildLanes(placed)
	tl.Steps = buildSteps(placed)
	return tl
}

// buildSteps is the flat run order: the same (start, dish, step) sort the
// lanes use, so the lyric flow and the Gantt cannot disagree.
func buildSteps(placed []placedStep) []domain.TimelineStep {
	order := append([]placedStep(nil), placed...)
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].start != order[j].start {
			return order[i].start < order[j].start
		}
		if order[i].dish != order[j].dish {
			return order[i].dish < order[j].dish
		}
		return order[i].index < order[j].index
	})
	out := make([]domain.TimelineStep, 0, len(order))
	for _, p := range order {
		out = append(out, domain.TimelineStep{
			RecipeID:    p.recipe.ID,
			RecipeName:  p.recipe.Name,
			Order:       p.dish,
			StepIndex:   p.step.OrderIndex,
			Servings:    p.servings,
			Instruction: p.step.Instruction,
			Start:       p.start,
			End:         p.end,
			Resource:    p.res,
			Resources:   p.step.Resources,
			Waiting:     IsWaiting(p.step.Resources),
		})
	}
	return out
}

// pickLongestChain picks the dish with the most planned minutes left. Ties go
// to the earlier dish, which keeps the plan deterministic.
func pickLongestChain(states []*dishState) *dishState {
	var best *dishState
	for _, st := range states {
		if st.next >= len(st.steps) {
			continue
		}
		if best == nil || st.remaining > best.remaining {
			best = st
		}
	}
	return best
}

// earliestFit finds the first minute at or after floor where the resource has
// room for dur minutes.
func earliestFit(placed []placedStep, resource string, stoveCount, floor, dur int) int {
	limit := capacity(resource, stoveCount)
	t := floor
	if limit <= 0 {
		return t
	}
	ends := make([]int, 0, limit+2)
	for guard := 0; guard < 4*maxClampMinutes; guard++ {
		ends = ends[:0]
		for _, p := range placed {
			if p.res != resource {
				continue
			}
			if p.start < t+dur && p.end > t {
				ends = append(ends, p.end)
			}
		}
		if len(ends) < limit {
			return t
		}
		sort.Ints(ends)
		t = ends[len(ends)-limit]
	}
	return t
}

// buildLanes assigns the placed steps to resource slots and names them. Steps
// are walked in run order so the lanes are stable for the same input.
func buildLanes(placed []placedStep) []domain.TimelineLane {
	order := append([]placedStep(nil), placed...)
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].start != order[j].start {
			return order[i].start < order[j].start
		}
		if order[i].dish != order[j].dish {
			return order[i].dish < order[j].dish
		}
		return order[i].index < order[j].index
	})

	type laneSlot struct {
		resource string
		start    int
		end      int
		steps    []domain.TimelineStep
	}
	byResource := map[string][]*laneSlot{}
	for _, p := range order {
		slots := byResource[p.res]
		var slot *laneSlot
		for _, cand := range slots {
			if cand.end <= p.start {
				slot = cand
				break
			}
		}
		if slot == nil {
			slot = &laneSlot{resource: p.res, start: p.start}
			slots = append(slots, slot)
			byResource[p.res] = slots
		}
		slot.steps = append(slot.steps, domain.TimelineStep{
			RecipeID:    p.recipe.ID,
			RecipeName:  p.recipe.Name,
			StepIndex:   p.step.OrderIndex,
			Servings:    p.servings,
			Instruction: p.step.Instruction,
			Start:       p.start,
			End:         p.end,
			Resource:    p.res,
			Resources:   p.step.Resources,
			Waiting:     IsWaiting(p.step.Resources),
		})
		if p.end > slot.end {
			slot.end = p.end
		}
		if p.start < slot.start {
			slot.start = p.start
		}
	}

	lanes := make([]domain.TimelineLane, 0, len(byResource))
	for _, resource := range []string{domain.ResourceStove, domain.ResourceBoard, domain.ResourceHand, domain.ResourceWaiting} {
		for i, slot := range byResource[resource] {
			lanes = append(lanes, domain.TimelineLane{
				Resource: resource,
				Slot:     i + 1,
				Label:    laneLabel(resource, i+1),
				Start:    slot.start,
				End:      slot.end,
				Steps:    slot.steps,
			})
		}
	}
	return lanes
}

// laneLabel is the Chinese name of a lane.
func laneLabel(resource string, slot int) string {
	name := "动手"
	switch resource {
	case domain.ResourceStove:
		name = "灶台"
	case domain.ResourceBoard:
		name = "案板"
	case domain.ResourceWaiting:
		name = "等待"
	}
	return name + " " + itoa(slot)
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
