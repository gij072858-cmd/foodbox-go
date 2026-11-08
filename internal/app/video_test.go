package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/videoproxy"
)

// fakeEmbedder stands in for the native mpv host so the player state machine
// can be exercised without a window or a player.
type fakeEmbedder struct {
	seeks   []float64
	seeksTo []float64
	pauses  []bool
	closed  bool
}

func (f *fakeEmbedder) Supported() bool { return true }
func (f *fakeEmbedder) Open(uintptr, videoproxy.Found, *videoproxy.Stream, ui.Rect) error {
	return nil
}
func (f *fakeEmbedder) Resize(ui.Rect)   {}
func (f *fakeEmbedder) SetPaused(v bool) { f.pauses = append(f.pauses, v) }
func (f *fakeEmbedder) Seek(s float64)   { f.seeks = append(f.seeks, s) }
func (f *fakeEmbedder) SeekTo(s float64) { f.seeksTo = append(f.seeksTo, s) }
func (f *fakeEmbedder) Close()           { f.closed = true }

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
	for _, label := range []string{"关闭视频", "暂停", "后退 10 秒", "前进 10 秒", "播放进度"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("player control %q is missing: %q", label, tt.Texts())
		}
	}
	if !tt.HasText("正在取这一页的直链…") {
		t.Errorf("player loading state missing: %q", tt.Texts())
	}
}

// Dragging the progress bar must send an absolute position to the player and
// move the app's own clock with it, not just nudge it by ten seconds.
func TestPlayerSliderSeeksToPosition(t *testing.T) {
	a := newTestApp()
	fake := &fakeEmbedder{}
	a.videoEmbed = fake
	a.videoPlayerOpen = true
	a.videoTitle = "测试"
	a.videoDur = 100 * time.Second
	a.videoPos = 10 * time.Second
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)

	track, ok := tt.Find("播放进度")
	if !ok {
		t.Fatalf("progress slider missing: %q", tt.Texts())
	}
	x, y := track.X+track.W*0.5, track.Y+track.H*0.5
	tt.Press(x, y)
	tt.Release(x, y)

	if len(fake.seeksTo) == 0 {
		t.Fatal("dragging the slider sent no absolute seek")
	}
	got := fake.seeksTo[len(fake.seeksTo)-1]
	if got < 45 || got > 55 {
		t.Errorf("absolute seek = %v s, want about 50", got)
	}
	if a.videoPos < 45*time.Second || a.videoPos > 55*time.Second {
		t.Errorf("position = %v, want the dragged position", a.videoPos)
	}
}

// mpv keeps reporting the old position for a moment after a seek; the thumb
// must not jump back while the seek is pending, and must follow once it lands.
func TestPlayerPositionWaitsForSeek(t *testing.T) {
	a := newTestApp()
	a.videoEmbed = &fakeEmbedder{}
	a.videoTitle = "测试"
	a.videoDur = 100 * time.Second
	a.beginSeek(50 * time.Second)

	a.onVideoPosition(10 * time.Second)
	if a.videoPos != 50*time.Second || !a.videoSeekPending {
		t.Errorf("stale position pulled the player back to %v", a.videoPos)
	}
	a.onVideoPosition(50200 * time.Millisecond)
	if a.videoSeekPending {
		t.Error("seek stayed pending after mpv reached the target")
	}
	if a.videoPos != 50200*time.Millisecond {
		t.Errorf("position = %v, want 50.2s", a.videoPos)
	}
}

// Seeking back from the end restarts playback instead of leaving the finished
// state on screen.
func TestPlayerSeekFromEndResumes(t *testing.T) {
	a := newTestApp()
	fake := &fakeEmbedder{}
	a.videoEmbed = fake
	a.videoTitle = "测试"
	a.videoDur = 100 * time.Second
	a.videoEnded, a.videoPaused = true, true

	a.beginSeek(25 * time.Second)

	if a.videoEnded || a.videoPaused {
		t.Errorf("ended=%v paused=%v after seeking back", a.videoEnded, a.videoPaused)
	}
	if len(fake.pauses) == 0 || fake.pauses[len(fake.pauses)-1] {
		t.Error("seeking back from the end did not unpause mpv")
	}
}

// The player's own fullscreen has no header: close and fullscreen-exit live
// in the bottom strip instead, and the progress bar stays reachable.
func TestPlayerFullscreenLayout(t *testing.T) {
	a := newTestApp()
	a.videoEmbed = &fakeEmbedder{}
	a.videoPlayerOpen = true
	a.videoPlayerFullscreen = true
	a.videoTitle = "测试"
	a.videoDur = 100 * time.Second
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	for _, label := range []string{"关闭视频", "退出全屏", "播放进度"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("fullscreen control %q is missing: %q", label, tt.Texts())
		}
	}
	if tt.HasText("菜式视频") {
		t.Errorf("fullscreen still draws the dialog header: %q", tt.Texts())
	}
}

// Escape leaves the player's fullscreen first and only closes on the next
// press.
func TestPlayerEscapeExitsFullscreenFirst(t *testing.T) {
	a := newTestApp()
	a.videoEmbed = &fakeEmbedder{}
	a.videoPlayerOpen, a.videoPlayerFullscreen = true, true

	a.playerEscape()
	if !a.videoPlayerOpen || a.videoPlayerFullscreen {
		t.Fatalf("escape closed the player or kept fullscreen: open=%v full=%v",
			a.videoPlayerOpen, a.videoPlayerFullscreen)
	}
	a.playerEscape()
	if a.videoPlayerOpen {
		t.Error("the second escape did not close the player")
	}
}

// A resolve that finishes after the user moved on must not start its stream,
// and closing the player must cancel a resolve still in flight.
func TestStaleResolveIsDroppedAndCloseCancels(t *testing.T) {
	a := newTestApp()
	a.videoSeq = 2
	a.videoPageURL = "new"

	staleRan := false
	a.postVideo(1, "old", func() { staleRan = true })
	if staleRan {
		t.Error("a post from an old resolve ran")
	}
	ran := false
	a.postVideo(2, "new", func() { ran = true })
	if !ran {
		t.Error("the current resolve's post did not run")
	}

	cancelled := false
	a.videoResolveCancel = func() { cancelled = true }
	a.closePlayer()
	if !cancelled {
		t.Error("closing the player did not cancel the resolve")
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
