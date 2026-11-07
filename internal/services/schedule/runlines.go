package schedule

import (
	"fmt"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// RunLines flattens the timeline's execution order into the lyric flow's
// lines. The timeline interleaves dishes by resource, so a two-dish table
// does not finish one dish before starting the other.
func RunLines(tl domain.Timeline) []domain.RunLine {
	lines := make([]domain.RunLine, 0, len(tl.Steps))
	for _, st := range tl.Steps {
		text := st.Instruction
		if text == "" {
			text = fmt.Sprintf("%s · 第 %d 步", st.RecipeName, st.StepIndex)
		}
		lines = append(lines, domain.RunLine{
			RecipeID:   st.RecipeID,
			RecipeName: st.RecipeName,
			StepIndex:  st.StepIndex,
			Text:       text,
			Minutes:    st.Minutes(),
			Waiting:    st.Waiting,
		})
	}
	return lines
}
