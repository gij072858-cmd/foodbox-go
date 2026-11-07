package design

import (
	"runtime"

	"github.com/egoist/mygo/ui"
)

// fontFamily is the family list FoodBox reads Chinese in. Windows' Segoe UI
// has no Han characters, so it must come after a Chinese family.
func fontFamily() string {
	if runtime.GOOS == "windows" {
		return "Noto Sans SC, Microsoft YaHei UI"
	}
	return ""
}

// ApplyTheme copies the system theme, applies FoodBox's tokens and sets it
// on the frame. mode is "auto" (follow the system), "light" or "dark";
// themeName is one of the Kero themes (unknown names fall back to paper).
// It returns the theme for the palette and widgets.
//
// A frame whose theme already carries these tokens skips SetTheme: setting
// it rebuilds the widgets that read the theme, and doing that every frame
// is pure cost. The check compares the palette's own values, so no native
// UI value is kept between frames.
func ApplyTheme(c *ui.Context, mode, themeName string) *ui.Theme {
	cur := c.Theme()
	dark := cur.Dark
	switch mode {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	p := PaletteFor(themeName, dark)
	if cur.Dark == dark && cur.Background == p.Bg && cur.Surface == p.Surface &&
		cur.Border == p.Border && cur.Text == p.Text && cur.Accent == p.Accent {
		return cur
	}
	t := *cur
	t.Dark = dark

	t.Background = p.Bg
	t.Surface = p.Surface
	t.SurfaceHover = p.Hover
	t.SurfacePressed = p.Press
	t.Border = p.Border
	t.Text = p.Text
	t.TextMuted = p.Muted
	t.Accent = p.Accent
	t.AccentHover = p.Accent
	t.AccentPressed = p.Accent
	t.AccentText = p.OnAccent
	t.Danger = p.Danger
	t.Warning = p.Warning
	t.Success = p.Success
	t.Selection = p.Accent.Alpha(0.24)
	t.Focus = p.Accent.Alpha(0.55)
	t.Scrollbar = p.Faint.Alpha(0.55)
	t.Inverse = p.Text
	t.InverseText = p.Bg
	// Built-in widgets share the card radius, so a search field and the
	// card it sits on do not disagree about corners.
	t.Radius = RadiusCard
	t.Spacing = 4
	t.FontSize = FontBody
	t.ScrollbarWidth = 8
	t.Font = fontFamily()

	c.SetTheme(&t)
	return &t
}
