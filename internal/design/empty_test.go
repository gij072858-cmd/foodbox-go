package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestEmptyStateIsTinted checks that a big "nothing here" panel wears the
// theme's accent instead of reading as a plain white card. The sample is
// taken in the card's padding, away from any text, and compared with the
// surface color — a stopped gradient would paint exactly the surface there.
// Both appearances are covered because dark derives its palette separately.
func TestEmptyStateIsTinted(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := PaletteFor(DefaultTheme, dark)
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Background(p.Bg).Padding(40).Children(func() {
				box := ui.Column(c).Key("empty-state").Label("empty-state")
				box.Children(func() {
					EmptyState(c, p, nil, "还没有内容", "换一个筛选，或开始记录。")
				})
			})
		}, 640, 360)
		tt.Frame()
		r, ok := tt.Find("empty-state")
		if !ok {
			t.Fatal("the empty-state panel was not found")
		}
		px := tt.Image().RGBAAt(int(r.X+40), int(r.Y+20))
		if channelDiff(px.R, p.Surface.R) <= 2 &&
			channelDiff(px.G, p.Surface.G) <= 2 &&
			channelDiff(px.B, p.Surface.B) <= 2 {
			t.Errorf("dark=%v: the empty state is a plain surface (%d,%d,%d)",
				dark, px.R, px.G, px.B)
		}
	}
}

func channelDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
