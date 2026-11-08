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

// The embedded player's panel is MyGo-drawn chrome around the native child:
// the close, seek and pause controls must all be present even while the
// direct link is still resolving.
func TestVideoPlayerOverlayControls(t *testing.T) {
	a := newTestApp()
	a.videoPlayerOpen = true
	a.videoBusy = true
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	for _, label := range []string{"关闭视频", "暂停", "后退 10 秒", "前进 10 秒"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("player control %q is missing: %q", label, tt.Texts())
		}
	}
	if !tt.HasText("正在取这一页的直链…") {
		t.Errorf("player loading state missing: %q", tt.Texts())
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
	for _, want := range []string{"内置播放器", "哔哩哔哩 SESSDATA", "不进备份"} {
		if !tt.HasText(want) {
			t.Errorf("the video section is missing %q: %q", want, tt.Texts())
		}
	}
}
