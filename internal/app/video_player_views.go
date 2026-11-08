package app

// The embedded video player: mpv paints into a native child window, and this
// file draws everything around it with MyGo. The child always sits above the
// painted content, so no control is placed on top of the video rectangle.

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/videoproxy"
)

// videoEmbedder is the platform's native player host. Windows creates a child
// HWND and talks to mpv over its JSON pipe; other platforms say unsupported
// and the app keeps the external-player/web fallback.
type videoEmbedder interface {
	Supported() bool
	Open(parent uintptr, player videoproxy.Found, stream *videoproxy.Stream, rect ui.Rect) error
	Resize(rect ui.Rect)
	SetPaused(bool)
	Seek(seconds float64)
	Close()
}

// playerClock is m:ss, the width a transport label needs.
func playerClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d / time.Second)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func (a *App) videoPlayerOverlay(c *ui.Context, p design.Palette) {
	if !a.videoPlayerOpen {
		return
	}
	w, h := min(a.winW-96, 1200), min(a.winH-96, 760)
	if w < 520 {
		w = a.winW - 24
	}
	if h < 360 {
		h = a.winH - 24
	}
	if w < 320 {
		w = 320
	}
	if h < 240 {
		h = 240
	}

	ui.DialogBase(c, &a.videoPlayerOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.55))
		panel.Children(func() {
			sheet := ui.Column(c).Width(w).Height(h).Padding(12).Gap(8).
				Radius(design.RadiusPane).Material(design.PaneMaterial(p)).
				Border(0.5, p.Border).Shadow(0, 24, 60, -20, p.Shadow)
			sheet.Children(func() {
				a.videoPlayerHeader(c, p)

				video := ui.Box(c).FillWidth().Grow(1).MinHeight(0).
					Radius(design.RadiusSmall).Background(ui.RGBA(0, 0, 0, 1))
				video.Children(func() {
					if a.videoBusy || a.videoTitle == "" {
						ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).
							Gap(4).Children(func() {
							ui.Text(c, "正在取这一页的直链…").FontSize(design.FontSmall).
								TextColor(ui.RGBA(255, 255, 255, 0.72))
							ui.Text(c, "解析成功后画面会出现在这里").FontSize(design.FontMeta).
								TextColor(ui.RGBA(255, 255, 255, 0.45))
						})
					} else if a.videoEnded {
						ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).
							Children(func() {
								ui.Text(c, "播放结束").FontSize(design.FontSmall).
									TextColor(ui.RGBA(255, 255, 255, 0.75))
							})
					}
				})

				// Bounds reports the previous frame; the native child is moved
				// from it every frame, so a resize or fullscreen follows with
				// one frame of lag.
				a.videoPlaceholder = video.Bounds()
				if a.videoPendingEmbed != nil && a.videoPlaceholder.W > 0 && a.videoPlaceholder.H > 0 {
					a.startEmbeddedPlayer(a.videoPageURL, a.videoPendingEmbed, a.videoPendingFound)
				}
				if a.videoEmbed != nil {
					if a.videoTitle == "" {
						a.videoEmbed.Resize(ui.Rect{})
					} else {
						a.videoEmbed.Resize(a.videoPlaceholder)
					}
				}

				a.videoPlayerControls(c, p)
			})
		})
	})
	// DialogBase closes on Escape or the backdrop; the native child must go
	// with it.
	if !a.videoPlayerOpen {
		a.closePlayer()
	}
}

func (a *App) videoPlayerHeader(c *ui.Context, p design.Palette) {
	title := a.videoTitle
	if title == "" {
		title = "菜式视频"
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(design.FontSubhead).FontWeight(700).
				TextColor(p.Text).SingleLine()
			sub := "哔哩哔哩 · 正在解析直链"
			if a.videoTitle != "" {
				sub = playerClock(a.videoPos) + " / " + playerClock(a.videoDur)
			}
			ui.Text(c, sub).FontSize(design.FontMeta).TextColor(p.Muted).SingleLine()
		})
		close := ui.ButtonBase(c.Key("video-player-close")).Size(28, 28).
			Radius(design.RadiusPill).Center().Label("关闭视频")
		design.Wash(close, p.Surface.Alpha(0), p.Hover)
		close.Children(func() {
			ui.Icon(c, &icons.Close).Size(14, 14).TextColor(p.Ink)
		})
		if close.Clicked() {
			a.videoPlayerOpen = false
		}
	})
}

func (a *App) videoPlayerControls(c *ui.Context, p design.Palette) {
	ready := a.videoTitle != "" && a.videoEmbed != nil
	iconButton := func(key string, icon *ui.SVG, label string, disabled bool, onClick func()) {
		b := ui.ButtonBase(c.Key(key)).Size(design.RunControlSize, design.RunControlSize).
			Radius(design.RadiusPill).Center().Label(label)
		design.Wash(b, p.Surface.Alpha(0), p.Hover)
		b.Children(func() {
			ui.Icon(c, icon).Size(17, 17).TextColor(p.Ink)
		})
		if disabled {
			b.Disabled(true).Opacity(0.35)
		} else if b.Clicked() {
			onClick()
		}
	}

	ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.Center).
		Gap(design.Sp(c, 2)).Children(func() {
		iconButton("video-seek-back", &icons.Prev, "后退 10 秒", !ready,
			func() { a.seekPlayer(-10) })

		playIcon, playLabel := &icons.Pause, "暂停"
		if a.videoPaused || a.videoEnded {
			playIcon, playLabel = &icons.Play, "播放"
		}
		if a.videoEnded {
			playLabel = "重播"
		}
		iconButton("video-play", playIcon, playLabel, !ready,
			func() { a.togglePlayerPause() })

		iconButton("video-seek-forward", &icons.Next, "前进 10 秒", !ready,
			func() { a.seekPlayer(10) })

		if a.win != nil {
			full := &icons.Fullscreen
			if a.win.IsFullScreen() {
				full = &icons.FullscreenExit
			}
			iconButton("video-fullscreen", full, "全屏", false,
				func() { a.togglePlayerFullscreen() })
		}
	})
}
