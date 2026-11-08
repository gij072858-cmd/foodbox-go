package app

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/videoproxy"
)

// A recipe's source link is a page on Bilibili or Xiaohongshu, not a media
// file. Two ways to watch it are kept, in this order:
//
//  1. resolve the page to a direct stream and hand it to mpv — what DownBit
//     does, with the endpoints iawia002/lux uses for Bilibili. This is the
//     one that answers "点开就是高画质的播放窗": no login wall, no site
//     chrome, the player's own full screen and quality menu. It takes a
//     moment to resolve, which is the price the user accepted.
//  2. show the page in an in-app web view. Kept whole, because Xiaohongshu
//     has no playurl endpoint to imitate, and because a machine with no mpv
//     on it still has to be able to watch the dish.
//
// MyGo's own Video element is not on this path: the framework decodes
// animated GIF only, and Bilibili's DASH answer is a split picture/sound pair
// that a GIF path cannot play whatever it was given.

// videoResolveTimeout bounds one resolve, so a phone hotspot that never
// answers cannot leave the player panel waiting forever.
const videoResolveTimeout = 60 * time.Second

// openVideo plays the page, or falls back to showing it.
func (a *App) openVideo(c *ui.Context, pageURL string) {
	if a.videoBusy || a.videoPlayerOpen || a.videoOpen {
		return
	}
	// Neither path can show anything without a window: an embedded page has
	// nowhere to go and an external player would be a result nobody sees. So
	// this asks before spending a network round trip, not after.
	if a.win == nil {
		a.notice = "视频页只在桌面窗口里打开"
		return
	}
	if !videoproxy.Supported(pageURL) {
		a.openWebView(c, pageURL)
		return
	}
	if a.videoEmbed == nil {
		a.videoEmbed = newVideoEmbedder(a)
	}

	// Every open gets a generation and its own cancellable context. Work that
	// finishes after the user moved on is dropped by postVideo.
	a.videoSeq++
	gen := a.videoSeq
	if a.videoResolveCancel != nil {
		a.videoResolveCancel()
		a.videoResolveCancel = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), videoResolveTimeout)
	a.videoResolveCancel = cancel

	if !a.videoEmbed.Supported() {
		// A platform without the native child window keeps the old external
		// player path; if that fails the web fallback still runs.
		a.videoBusy = true
		a.videoPending = pageURL
		a.videoPageURL = pageURL
		a.notice = "正在取这一页的直链…"
		go a.playExternal(ctx, cancel, gen, pageURL)
		return
	}
	a.videoPlayerOpen = true
	a.videoBusy = true
	a.videoPageURL = pageURL
	a.videoPending = ""
	a.videoTitle = ""
	a.videoPaused, a.videoEnded = false, false
	a.videoPos, a.videoDur = 0, 0
	a.resetPlayerSeek()
	a.videoPlayerFullscreen = false
	a.notice = "正在取这一页的直链…"
	go a.playEmbedded(ctx, cancel, gen, pageURL)
}

// postVideo runs fn on the main thread only if the resolve it belongs to is
// still the current one. This is what keeps a late answer for an old page
// from starting a stream the user already walked away from.
func (a *App) postVideo(gen uint64, pageURL string, fn func()) {
	a.post(func() {
		if a.videoSeq != gen || a.videoPageURL != pageURL {
			return
		}
		fn()
	})
}

// playEmbedded resolves the stream, then hands it to the main thread, where
// the native child window is created. Direct URLs expire within minutes, so
// nothing here is worth caching: the next click resolves again.
func (a *App) playEmbedded(ctx context.Context, cancel context.CancelFunc, gen uint64, pageURL string) {
	defer cancel()

	sessdata, _ := a.biliCookie()
	stream, err := videoproxy.Resolve(ctx, pageURL, sessdata)
	if err != nil {
		a.postVideo(gen, pageURL, func() { a.videoGaveUp(err) })
		return
	}
	found := videoproxy.FindPlayer(a.dataDir())
	if found.Kind != videoproxy.PlayerMPV {
		a.postVideo(gen, pageURL, func() { a.videoGaveUp(videoproxy.ErrNoPlayer) })
		return
	}
	a.postVideo(gen, pageURL, func() {
		a.startEmbeddedPlayer(gen, pageURL, stream, found)
	})
}

// startEmbeddedPlayer creates the native child and starts mpv. It runs on the
// main thread because a child window created anywhere else belongs to a
// thread with no message loop. The overlay lays out before a resolve finishes,
// so the rectangle is normally ready; if it is not, the stream waits for the
// next frame.
func (a *App) startEmbeddedPlayer(gen uint64, pageURL string, stream *videoproxy.Stream, found videoproxy.Found) {
	if !a.videoPlayerOpen || a.videoEmbed == nil ||
		a.videoSeq != gen || a.videoPageURL != pageURL {
		return
	}
	rect := a.videoPlaceholder
	if rect.W <= 0 || rect.H <= 0 {
		a.videoPendingEmbed, a.videoPendingFound = stream, found
		a.videoPendingGen, a.videoPendingPage = gen, pageURL
		return
	}
	if err := a.videoEmbed.Open(a.win.NativeHandle(), found, stream, rect); err != nil {
		a.videoGaveUp(err)
		return
	}
	a.videoPendingEmbed = nil
	a.videoPendingFound = videoproxy.Found{}
	a.videoPendingGen, a.videoPendingPage = 0, ""
	a.videoBusy = false
	a.videoTitle = stream.Title
	a.videoPageURL = pageURL
	a.videoPos, a.videoDur = 0, 0
	a.resetPlayerSeek()
}

// playExternal is the non-Windows path: resolve, then start mpv/VLC in its
// own window. The embedded player does not use it.
func (a *App) playExternal(ctx context.Context, cancel context.CancelFunc, gen uint64, pageURL string) {
	defer cancel()

	sessdata, _ := a.biliCookie()
	stream, err := videoproxy.Resolve(ctx, pageURL, sessdata)
	if err == nil {
		found := videoproxy.FindPlayer(a.dataDir())
		if found.Kind == videoproxy.PlayerNone {
			err = videoproxy.ErrNoPlayer
		} else {
			var cmd *exec.Cmd
			cmd, err = videoproxy.Launch(context.Background(), found, *stream)
			if err == nil {
				a.postVideo(gen, pageURL, func() {
					a.videoStarted(pageURL, stream.Title, found.Kind, cmd)
				})
				return
			}
		}
	}
	a.postVideo(gen, pageURL, func() { a.videoGaveUp(err) })
}

// videoStarted records the external player process, and waits on it so that
// closing it clears the app's idea of what is playing.
func (a *App) videoStarted(pageURL, title string, kind videoproxy.Player, cmd *exec.Cmd) {
	a.videoBusy, a.videoPending = false, ""
	a.videoCmd, a.videoCmdURL = cmd, pageURL
	a.notice = "已用 " + kind.String() + " 打开：" + title
	go func() {
		_ = cmd.Wait()
		a.post(func() {
			if a.videoCmd == cmd {
				a.videoCmd, a.videoCmdURL = nil, ""
			}
		})
	}()
}

// videoGaveUp is the honest path: say why, then let the next frame open the
// page instead, so the button the user pressed still does something.
func (a *App) videoGaveUp(cause error) {
	a.videoBusy = false
	a.videoPendingEmbed = nil
	a.videoPendingFound = videoproxy.Found{}
	a.videoPendingGen, a.videoPendingPage = 0, ""
	a.videoPlayerFullscreen = false
	a.restoreFullscreen()
	if a.videoPlayerOpen {
		a.videoPlayerOpen = false
		if a.videoEmbed != nil {
			a.videoEmbed.Close()
		}
	}
	if a.videoPageURL != "" {
		a.videoPending = a.videoPageURL
	}
	if errors.Is(cause, videoproxy.ErrNoPlayer) {
		a.notice = "视频预览需要本机 mpv（设置 › 视频播放）；已改用应用内网页播放"
		return
	}
	a.notice = "取直链没成，改用应用内网页播放：" + cause.Error()
}

// closePlayer tears down the embedded player, restoring the window mode. It
// also cancels a resolve still in flight, so walking away cannot start the
// stream a moment later.
func (a *App) closePlayer() {
	a.videoSeq++
	if a.videoResolveCancel != nil {
		a.videoResolveCancel()
		a.videoResolveCancel = nil
	}
	if a.videoEmbed != nil {
		a.videoEmbed.Close()
	}
	a.videoPlayerOpen = false
	a.videoBusy = false
	a.videoPendingEmbed = nil
	a.videoPendingFound = videoproxy.Found{}
	a.videoPendingGen, a.videoPendingPage = 0, ""
	a.videoPending = ""
	a.videoPageURL = ""
	a.videoTitle = ""
	a.videoPaused, a.videoEnded = false, false
	a.videoPos, a.videoDur = 0, 0
	a.resetPlayerSeek()
	a.videoPlaceholder = ui.Rect{}
	a.videoPlayerFullscreen = false
	a.restoreFullscreen()
}

// togglePlayerPause flips mpv's pause state; a finished stream restarts.
func (a *App) togglePlayerPause() {
	if a.videoEmbed == nil || a.videoTitle == "" {
		return
	}
	if a.videoEnded {
		a.videoEnded = false
		a.videoPaused = false
		a.seekPlayerTo(0)
		a.videoEmbed.SetPaused(false)
		return
	}
	a.videoPaused = !a.videoPaused
	a.videoEmbed.SetPaused(a.videoPaused)
}

// seekPlayer moves mpv by seconds, relative to where it is.
func (a *App) seekPlayer(seconds float64) {
	if a.videoEmbed == nil || a.videoTitle == "" {
		return
	}
	a.seekPlayerTo(a.videoPos + time.Duration(seconds*float64(time.Second)))
}

// seekPlayerTo seeks to an absolute position, clamping to the stream.
func (a *App) seekPlayerTo(target time.Duration) {
	if a.videoEmbed == nil || a.videoTitle == "" {
		return
	}
	a.beginSeek(target)
	a.sendPlayerSeek()
}

// beginSeek records an optimistic position and arms the pending state, so
// mpv's time-pos reports from before the seek cannot pull the thumb back.
func (a *App) beginSeek(target time.Duration) {
	if target < 0 {
		target = 0
	}
	if a.videoDur > 0 && target > a.videoDur {
		target = a.videoDur
	}
	a.videoPos = target
	a.videoSeekTarget = target
	a.videoSeekPending = true
	a.videoSeekAt = time.Now()
	if a.videoDur > 0 {
		a.videoSlider = clampRatio(float64(target) / float64(a.videoDur))
	}
	if a.videoEnded {
		a.videoEnded = false
		a.videoPaused = false
		if a.videoEmbed != nil {
			a.videoEmbed.SetPaused(false)
		}
	}
}

// sendPlayerSeek sends the pending target and remembers what was sent.
func (a *App) sendPlayerSeek() {
	if a.videoEmbed == nil {
		return
	}
	a.videoEmbed.SeekTo(a.videoSeekTarget.Seconds())
	a.videoSeekSentAt = time.Now()
	a.videoSeekSentTarget = a.videoSeekTarget
}

// onVideoPosition accepts mpv's time-pos. While a seek is pending, a sample
// still near the old position is ignored; once mpv reaches the target, or
// the seek has had time to land, the player follows mpv again.
func (a *App) onVideoPosition(pos time.Duration) {
	if a.videoSeekPending {
		delta := pos - a.videoSeekTarget
		if delta < 0 {
			delta = -delta
		}
		if delta > 750*time.Millisecond && time.Since(a.videoSeekAt) < 1500*time.Millisecond {
			return
		}
		a.videoSeekPending = false
	}
	a.videoPos = pos
}

// resetPlayerSeek clears the slider and its pending seek for a new stream.
func (a *App) resetPlayerSeek() {
	a.videoSlider = 0
	a.videoSliderHeld = false
	a.videoSeekPending = false
	a.videoSeekTarget = 0
	a.videoSeekAt = time.Time{}
	a.videoSeekSentAt = time.Time{}
	a.videoSeekSentTarget = 0
}

// enterPlayerFullscreen fills the window with the player; exitPlayerFullscreen
// puts the window back the way it was. The app-window toggle is not enough:
// the dialog has to grow with it, which videoPlayerOverlay does from this flag.
func (a *App) enterPlayerFullscreen() {
	a.videoPlayerFullscreen = true
	if a.win == nil {
		return
	}
	if !a.videoFullscreenSaved {
		a.videoPrevFullscreen = a.win.IsFullScreen()
		a.videoFullscreenSaved = true
	}
	a.win.SetFullScreen(true)
}

func (a *App) exitPlayerFullscreen() {
	a.videoPlayerFullscreen = false
	a.restoreFullscreen()
}

// togglePlayerFullscreen toggles the player's own fullscreen.
func (a *App) togglePlayerFullscreen() {
	if a.videoPlayerFullscreen {
		a.exitPlayerFullscreen()
		return
	}
	a.enterPlayerFullscreen()
}

// playerEscape leaves the player's fullscreen first, and closes it otherwise.
func (a *App) playerEscape() {
	if a.videoPlayerFullscreen {
		a.exitPlayerFullscreen()
		return
	}
	a.closePlayer()
}

// enterFullscreen saves the window mode once, then fills the screen for the
// web fallback.
func (a *App) enterFullscreen() {
	if a.win == nil {
		return
	}
	if !a.videoFullscreenSaved {
		a.videoPrevFullscreen = a.win.IsFullScreen()
		a.videoFullscreenSaved = true
	}
	a.win.SetFullScreen(true)
}

// restoreFullscreen puts the window back the way it was before a video.
func (a *App) restoreFullscreen() {
	if !a.videoFullscreenSaved {
		return
	}
	a.videoFullscreenSaved = false
	if a.win != nil {
		a.win.SetFullScreen(a.videoPrevFullscreen)
	}
}

// stopVideo ends every player this app started.
func (a *App) stopVideo() {
	a.closePlayer()
	a.closeVideo()
	if a.videoCmd == nil {
		return
	}
	cmd := a.videoCmd
	a.videoCmd, a.videoCmdURL = nil, ""
	go func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
}

// dataDir is where the app keeps its own files, and so where a player it
// installed itself would live.
func (a *App) dataDir() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	return dir
}

// openWebView shows url in the in-app web fallback, full screen: a page that
// cannot be resolved is at least given the whole window.
func (a *App) openWebView(c *ui.Context, url string) {
	if a.win == nil {
		a.notice = "视频页只在桌面窗口里打开"
		return
	}
	if a.videoView != nil && a.videoURL == url && !a.videoView.IsDestroyed() {
		a.videoOpen = true
		a.enterFullscreen()
		return
	}
	a.closeVideo()
	a.enterFullscreen()
	view, err := a.win.NewWebView(mygo.WebViewOptions{URL: url})
	if err != nil {
		a.restoreFullscreen()
		a.notice = "打不开视频页：" + err.Error()
		c.OpenURL(url)
		return
	}
	a.videoView = view
	a.videoURL = url
	a.videoOpen = true
}

// closeVideo hides and destroys the web view; a web view kept alive would
// keep decoding behind the page.
func (a *App) closeVideo() {
	a.videoOpen = false
	if a.videoView != nil {
		a.videoView.Destroy()
		a.videoView = nil
	}
	a.videoURL = ""
	a.restoreFullscreen()
}

// videoOverlay is the sheet: a glass frame with the page in it. DialogBase
// gives the dim backdrop, Escape and the outside click for free. It also
// carries the one job left on this thread: a resolution that failed has to
// open the page with a frame's context in hand.
func (a *App) videoOverlay(c *ui.Context, p design.Palette) {
	if a.videoPending != "" && !a.videoBusy {
		page := a.videoPending
		a.videoPending = ""
		a.openWebView(c, page)
	}
	if !a.videoOpen || a.videoView == nil {
		return
	}
	ui.DialogBase(c, &a.videoOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.45))
		panel.Children(func() {
			w, h := a.winW-24, a.winH-24
			if w < 600 {
				w = 600
			}
			if h < 420 {
				h = 420
			}
			sheet := ui.Column(c).Width(w).Height(h).Padding(12).Gap(8).
				Radius(design.RadiusPane).Material(design.PaneMaterial(p)).
				Border(0.5, p.Border).Shadow(0, 24, 60, -20, p.Shadow)
			sheet.Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "菜式视频").FontSize(design.FontSubhead).
						FontWeight(700).TextColor(p.Text).Grow(1)
					ui.Text(c, a.videoURL).FontSize(design.FontMeta).
						TextColor(p.Muted).SingleLine().MinWidth(0)
					if design.Button(c, p, "video-close", "关闭",
						design.ButtonSecondary).Clicked() {
						a.videoOpen = false
					}
				})
				ui.WebView(c, a.videoView).Grow(1).Radius(design.RadiusSmall)
			})
		})
	})
	// DialogBase closes on Escape or the backdrop; the native view must go
	// with it.
	if !a.videoOpen {
		a.closeVideo()
	}
}

// biliCookieKey is the credential-store key for the Bilibili login cookie.
// It is not in prefs: the data rule says a backup carries no secrets, and a
// SESSDATA is a person's account.
const biliCookieKey = "bilibili-sessdata"

// biliCookie reads the stored SESSDATA. A missing cookie is not an error:
// Bilibili still serves the qualities a stranger may see.
func (a *App) biliCookie() (string, bool) {
	v, err := mygo.Secrets.Get(biliCookieKey)
	if err != nil || len(v) == 0 {
		return "", false
	}
	return string(v), true
}

// saveBiliCookie stores the pasted SESSDATA in the system credential store,
// taking just its value out of whatever was copied out of a browser.
func (a *App) saveBiliCookie(cookie string) error {
	cookie = trimTo(cookie, ";", " ", "\n", "\t")
	if cookie == "" {
		return nil
	}
	return mygo.Secrets.Set(biliCookieKey, []byte(cookie))
}

// clearBiliCookie forgets it.
func (a *App) clearBiliCookie() {
	_ = mygo.Secrets.Delete(biliCookieKey)
}

// trimTo cuts s at the earliest of cuts, so a pasted cookie header keeps only
// its value.
func trimTo(s string, cuts ...string) string {
	s = strings.TrimSpace(s)
	cut := len(s)
	for _, c := range cuts {
		if i := strings.Index(s, c); i >= 0 && i < cut {
			cut = i
		}
	}
	return strings.TrimSpace(s[:cut])
}

// videoProbeState is the settings page's cached I/O. A zero value means the
// probe has not run yet.
type videoProbeState struct {
	found    videoproxy.Found
	foundOK  bool
	cookie   bool
	cookieOK bool
}

// probeVideo reads what the settings page reports: which player could play,
// and whether a login cookie is stored. It is called on bootstrap's
// goroutine, never while a frame is built.
func (a *App) probeVideo() (videoproxy.Found, bool) {
	found := videoproxy.FindPlayer(a.dataDir())
	_, have := a.biliCookie()
	return found, have
}
