package app

import "github.com/egoist/mygo/ui"

// Icons are parsed once, never in the view.
type iconSet struct {
	Logo, Pantry, Recipe, Sichu, Schedule, Feed, Profile, Settings     ui.SVG
	Search, Plus, Back, Chevron, ChevronDown, Clock, Spark, Check      ui.SVG
	Help, Play, Pause, Prev, Next, Edit, Trash, Calendar, Chart, Flame ui.SVG
	Leaf, Bowl, Key, Star, Close, Info, Alert, Download, Upload        ui.SVG
	Fish, Egg                                                          ui.SVG
}

func parseIcon(body string) ui.SVG {
	return *ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var icons = iconSet{
	Logo:        parseIcon(`<path d="M4 8h16l-1.5 11a2 2 0 0 1-2 1.8H7.5a2 2 0 0 1-2-1.8z"/><path d="M8 8V6a4 4 0 0 1 8 0v2"/>`),
	Pantry:      parseIcon(`<path d="M3 7l9-4 9 4v10l-9 4-9-4z"/><path d="M3 7l9 4 9-4"/><path d="M12 11v10"/>`),
	Recipe:      parseIcon(`<path d="M4 5a2 2 0 0 1 2-2h12v18H6a2 2 0 0 1-2-2z"/><path d="M8 7h6"/><path d="M8 11h6"/>`),
	Sichu:       parseIcon(`<path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z"/><path d="M19 3v4"/><path d="M21 5h-4"/>`),
	Schedule:    parseIcon(`<circle cx="12" cy="13" r="8"/><path d="M12 9v4l3 2"/><path d="M9 2h6"/>`),
	Feed:        parseIcon(`<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18"/><path d="M8 3v4"/><path d="M16 3v4"/>`),
	Profile:     parseIcon(`<circle cx="12" cy="8" r="4"/><path d="M4 21c0-4 4-6 8-6s8 2 8 6"/>`),
	Settings:    parseIcon(`<path d="M4 6h16"/><path d="M4 12h16"/><path d="M4 18h16"/><circle cx="9" cy="6" r="2"/><circle cx="15" cy="12" r="2"/><circle cx="8" cy="18" r="2"/>`),
	Search:      parseIcon(`<circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/>`),
	Plus:        parseIcon(`<path d="M12 5v14"/><path d="M5 12h14"/>`),
	Back:        parseIcon(`<path d="M19 12H5"/><path d="M12 19l-7-7 7-7"/>`),
	Chevron:     parseIcon(`<path d="M9 18l6-6-6-6"/>`),
	ChevronDown: parseIcon(`<path d="M6 9l6 6 6-6"/>`),
	Clock:       parseIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`),
	Spark:       parseIcon(`<path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z"/>`),
	Check:       parseIcon(`<path d="M20 6L9 17l-5-5"/>`),

	Help:     parseIcon(`<circle cx="12" cy="12" r="9"/><path d="M9.5 9a2.5 2.5 0 1 1 4 2c-.9.7-1.5 1.2-1.5 2.5"/><path d="M12 17h.01"/>`),
	Play:     parseIcon(`<path d="M8 5l11 7-11 7z"/>`),
	Pause:    parseIcon(`<path d="M9 5v14"/><path d="M15 5v14"/>`),
	Prev:     parseIcon(`<path d="M19 5v14l-9-7z"/><path d="M5 5v14"/>`),
	Next:     parseIcon(`<path d="M5 5v14l9-7z"/><path d="M19 5v14"/>`),
	Edit:     parseIcon(`<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>`),
	Trash:    parseIcon(`<path d="M4 7h16"/><path d="M9 7V5h6v2"/><path d="M6 7l1 13h10l1-13"/>`),
	Calendar: parseIcon(`<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18"/><path d="M8 3v4"/><path d="M16 3v4"/>`),
	Chart:    parseIcon(`<path d="M4 20V10"/><path d="M10 20V4"/><path d="M16 20v-7"/><path d="M22 20H2"/>`),
	Flame:    parseIcon(`<path d="M12 3c3 4 5 6 5 9a5 5 0 0 1-10 0c0-3 2-5 5-9z"/>`),
	Leaf:     parseIcon(`<path d="M4 20c8 0 16-6 16-16C10 4 4 10 4 20z"/><path d="M4 20l8-8"/>`),
	Bowl:     parseIcon(`<path d="M3 11h18a9 9 0 0 1-18 0z"/><path d="M7 11V7"/><path d="M12 11V5"/><path d="M17 11V7"/>`),
	Key:      parseIcon(`<circle cx="8" cy="15" r="4"/><path d="M11 12l9-9"/><path d="M17 3h4v4"/>`),
	Star:     parseIcon(`<path d="M12 3l2.6 5.6 6.1.8-4.5 4.2 1.2 6-5.4-3-5.4 3 1.2-6L3.3 9.4l6.1-.8z"/>`),
	Close:    parseIcon(`<path d="M6 6l12 12"/><path d="M18 6L6 18"/>`),
	Info:     parseIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 8h.01"/>`),
	Alert:    parseIcon(`<path d="M12 3l9 17H3z"/><path d="M12 9v5"/><path d="M12 17h.01"/>`),
	Download: parseIcon(`<path d="M12 4v11"/><path d="M7 10l5 5 5-5"/><path d="M4 20h16"/>`),
	Upload:   parseIcon(`<path d="M12 20V9"/><path d="M7 14l5-5 5 5"/><path d="M4 4h16"/>`),

	// Dish marks for the recipe groups the navigation set has no shape for.
	Fish: parseIcon(`<path d="M21 12c-2-3.4-5.3-5-8.5-5S6 8.6 4 12c2 3.4 5.3 5 8.5 5s6.5-1.6 8.5-5z"/><path d="M5 12 2 8.5"/><path d="M5 12 2 15.5"/><path d="M8 10.8h.01"/>`),
	Egg:  parseIcon(`<circle cx="12" cy="12" r="8"/><circle cx="12" cy="12" r="3"/>`),
}
