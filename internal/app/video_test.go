package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The cookie a person pastes is usually a whole header, or a list of them.
// Only the one value is worth storing, and storing more would send it on
// every request.
func TestTrimToCookieValue(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a1b2c3", "a1b2c3"},
		{"  a1b2c3  ", "a1b2c3"},
		{"SESSDATA=a1b2c3; buvid3=xyz", "SESSDATA=a1b2c3"},
		{"a1b2c3\n", "a1b2c3"},
		{"a1b2c3\textra", "a1b2c3"},
		{"", ""},
	} {
		if got := trimTo(c.in, ";", " ", "\n", "\t"); got != c.want {
			t.Errorf("trimTo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// With no window there is nothing to embed a page in, and no player worth
// waiting for: the click must say so instead of hanging on a resolve that
// can only end in the browser.
func TestOpenVideoWithoutWindowSaysSo(t *testing.T) {
	a := newTestApp()
	a.openVideo(nil, "https://www.bilibili.com/video/BV1GJ411x7h7/")
	if a.notice == "" {
		t.Error("the click was silent")
	}
	if a.videoBusy {
		a.stopVideo()
		t.Error("a resolve was started that no window could ever show")
	}
}

// The settings page must state, in the user's terms, which player will be
// used and where the login cookie lives.
func TestVideoSettingsSection(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(400 * time.Millisecond)
	for _, want := range []string{"外部播放器", "哔哩哔哩 SESSDATA", "不进备份"} {
		if !tt.HasText(want) {
			t.Errorf("the video section is missing %q: %q", want, tt.Texts())
		}
	}
}
