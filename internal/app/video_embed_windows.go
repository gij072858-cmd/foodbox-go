//go:build windows

package app

// Windows hosts the embedded player as a child HWND of the app window. mpv
// renders into that window with --wid; because the window belongs to this
// process, mpv receives no input on it, so every control is sent over mpv's
// JSON IPC pipe. The child is created after MyGo's surface and kept at the
// top of the sibling z-order; all MyGo chrome stays outside its rectangle.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"

	"github.com/gij072858-cmd/foodbox-go/internal/videoproxy"
)

const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsClipSiblings = 0x04000000
	wsClipChildren = 0x02000000
	ssBlackRect    = 0x00000004

	swHide = 0
	swShow = 5

	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
	hwndTop       = 0
	gwlpWndProc   = ^uintptr(3)
	wmKeyDown     = 0x0100
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	vkEscape      = 0x1B
	vkSpace       = 0x20
	vkLeft        = 0x25
	vkRight       = 0x27
	vkF           = 0x46
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procSetWindowPos      = user32.NewProc("SetWindowPos")
	procShowWindow        = user32.NewProc("ShowWindow")
	procGetDpiForWindow   = user32.NewProc("GetDpiForWindow")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW   = user32.NewProc("CallWindowProcW")
	procSetFocus          = user32.NewProc("SetFocus")
)

var mpvPipeCounter uint64

// mpvEmbedder is one running mpv process and the child window it draws in.
type mpvEmbedder struct {
	app    *App
	player videoproxy.Found
	stream *videoproxy.Stream

	parent  uintptr
	hwnd    uintptr
	oldProc uintptr
	procCb  uintptr

	cmd      *exec.Cmd
	pipeName string

	writeMu sync.Mutex
	pipe    *os.File
	closed  atomic.Bool
	played  atomic.Bool
	// session changes on every Open and Close so a goroutine left over from
	// a previous player cannot report into the new one.
	session atomic.Uint64
}

func newVideoEmbedder(a *App) videoEmbedder {
	return &mpvEmbedder{app: a}
}

func (*mpvEmbedder) Supported() bool { return true }

func (p *mpvEmbedder) Open(parent uintptr, player videoproxy.Found, stream *videoproxy.Stream, rect ui.Rect) error {
	if p.hwnd != 0 {
		p.Close()
	}
	p.parent = parent
	p.player = player
	p.stream = stream
	p.closed.Store(false)
	p.played.Store(false)
	gen := p.session.Add(1)
	if err := p.createWindow(); err != nil {
		return err
	}
	p.pipeName = fmt.Sprintf(`\\.\pipe\foodbox-mpv-%d-%d`,
		os.Getpid(), atomic.AddUint64(&mpvPipeCounter, 1))
	args := videoproxy.EmbeddedMPVArgs(player.Path, p.hwnd, p.pipeName, *stream)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		p.destroyWindow()
		return fmt.Errorf("播放器起不来：%w", err)
	}
	p.cmd = cmd
	p.resize(rect)
	procShowWindow.Call(p.hwnd, uintptr(swShow))
	go p.connect(gen)
	go p.wait(gen, cmd)
	return nil
}

// createWindow makes a plain STATIC child; mpv owns its pixels. The new child
// is inserted above MyGo's existing surface in the sibling z-order.
func (p *mpvEmbedder) createWindow() error {
	className, err := syscall.UTF16PtrFromString("STATIC")
	if err != nil {
		return err
	}
	style := uintptr(wsChild | wsVisible | wsClipSiblings | wsClipChildren | ssBlackRect)
	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		style,
		0, 0, 1, 1,
		p.parent,
		0, 0, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("视频窗口创建失败：%v", callErr)
	}
	p.hwnd = hwnd
	p.procCb = syscall.NewCallback(p.wndProc)
	old, _, _ := procSetWindowLongPtrW.Call(hwnd, gwlpWndProc, p.procCb)
	p.oldProc = old
	procSetWindowPos.Call(hwnd, hwndTop, 0, 0, 1, 1, swpNoActivate|swpShowWindow)
	procShowWindow.Call(hwnd, swHide)
	return nil
}

func (p *mpvEmbedder) destroyWindow() {
	if p.hwnd != 0 {
		procDestroyWindow.Call(p.hwnd)
		p.hwnd = 0
	}
}

// wndProc gives the child the input mpv cannot receive in a foreign process:
// click pauses, arrow keys seek, Escape closes, F toggles fullscreen.
func (p *mpvEmbedder) wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	app := p.app
	switch msg {
	case wmKeyDown:
		switch wParam {
		case vkSpace:
			app.post(func() { app.togglePlayerPause() })
			return 0
		case vkLeft:
			app.post(func() { app.seekPlayer(-10) })
			return 0
		case vkRight:
			app.post(func() { app.seekPlayer(10) })
			return 0
		case vkEscape:
			app.post(func() { app.closePlayer() })
			return 0
		case vkF, 'f':
			app.post(func() { app.togglePlayerFullscreen() })
			return 0
		}
	case wmLButtonDown:
		procSetFocus.Call(hwnd)
		return 0
	case wmLButtonUp:
		app.post(func() { app.togglePlayerPause() })
		return 0
	}
	ret, _, _ := procCallWindowProcW.Call(p.oldProc, hwnd, msg, wParam, lParam)
	return ret
}

func (p *mpvEmbedder) Resize(rect ui.Rect) { p.resize(rect) }

func (p *mpvEmbedder) resize(rect ui.Rect) {
	if p.hwnd == 0 {
		return
	}
	if rect.W <= 0 || rect.H <= 0 {
		procShowWindow.Call(p.hwnd, swHide)
		return
	}
	scale := dpiScale(p.parent)
	x := int32(math.Round(float64(rect.X) * scale))
	y := int32(math.Round(float64(rect.Y) * scale))
	w := int32(math.Round(float64(rect.W) * scale))
	h := int32(math.Round(float64(rect.H) * scale))
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	procSetWindowPos.Call(p.hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		swpNoZOrder|swpNoActivate|swpShowWindow)
}

func dpiScale(hwnd uintptr) float64 {
	if err := procGetDpiForWindow.Find(); err != nil {
		return 1
	}
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

// connect waits for mpv's IPC pipe, then starts observing the properties the
// panel shows.
func (p *mpvEmbedder) connect(gen uint64) {
	pipe, err := dialPipe(p.pipeName, 5*time.Second)
	if err != nil {
		if !p.closed.Load() && p.session.Load() == gen {
			p.app.post(func() { p.app.embedFailed(fmt.Errorf("连接播放器失败：%w", err)) })
		}
		return
	}
	if p.closed.Load() || p.session.Load() != gen {
		_ = pipe.Close()
		return
	}
	p.writeMu.Lock()
	p.pipe = pipe
	p.writeMu.Unlock()
	p.send("observe_property", 1, "pause")
	p.send("observe_property", 2, "time-pos")
	p.send("observe_property", 3, "duration")
	p.send("observe_property", 4, "eof-reached")
	go p.readLoop(gen, pipe)
}

func dialPipe(path string, timeout time.Duration) (*os.File, error) {
	deadline := time.Now().Add(timeout)
	for {
		h, err := windows.CreateFile(windows.StringToUTF16Ptr(path),
			windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
			windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			return os.NewFile(uintptr(h), path), nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (p *mpvEmbedder) readLoop(gen uint64, pipe *os.File) {
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var msg struct {
			Event  string          `json:"event"`
			Name   string          `json:"name"`
			Data   json.RawMessage `json:"data"`
			Reason string          `json:"reason"`
		}
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			continue
		}
		if p.session.Load() != gen {
			return
		}
		switch msg.Event {
		case "property-change":
			p.property(gen, msg.Name, msg.Data)
		case "end-file":
			switch msg.Reason {
			case "eof":
				p.app.post(func() {
					if p.session.Load() != gen {
						return
					}
					p.app.videoEnded = true
					p.app.videoPaused = true
				})
			case "error":
				if !p.closed.Load() {
					p.app.post(func() {
						if p.session.Load() != gen {
							return
						}
						p.app.embedFailed(fmt.Errorf("视频流播放失败"))
					})
				}
			}
		}
	}
}

func (p *mpvEmbedder) property(gen uint64, name string, data json.RawMessage) {
	switch name {
	case "pause":
		var v bool
		if json.Unmarshal(data, &v) == nil {
			value := v
			p.app.post(func() {
				if p.session.Load() == gen {
					p.app.videoPaused = value
				}
			})
		}
	case "time-pos":
		var v float64
		if json.Unmarshal(data, &v) == nil {
			p.played.Store(v > 0)
			pos := time.Duration(v * float64(time.Second))
			p.app.post(func() {
				if p.session.Load() == gen {
					p.app.videoPos = pos
				}
			})
		}
	case "duration":
		var v float64
		if json.Unmarshal(data, &v) == nil {
			dur := time.Duration(v * float64(time.Second))
			p.app.post(func() {
				if p.session.Load() == gen {
					p.app.videoDur = dur
				}
			})
		}
	case "eof-reached":
		var v bool
		if json.Unmarshal(data, &v) == nil && v {
			p.app.post(func() {
				if p.session.Load() != gen {
					return
				}
				p.app.videoEnded = true
				p.app.videoPaused = true
			})
		}
	}
}

func (p *mpvEmbedder) SetPaused(paused bool) { p.send("set_property", "pause", paused) }
func (p *mpvEmbedder) Seek(seconds float64)  { p.send("seek", seconds, "relative") }

func (p *mpvEmbedder) send(command ...any) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.pipe == nil {
		return
	}
	b, err := json.Marshal(map[string]any{"command": command})
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = p.pipe.Write(b)
}

// wait reaps mpv and, if the app did not close it, hands the failure back to
// the main thread. The panel either falls back to the web view or says the
// player exited.
func (p *mpvEmbedder) wait(gen uint64, cmd *exec.Cmd) {
	err := cmd.Wait()
	if p.closed.Load() || p.session.Load() != gen {
		return
	}
	played := p.played.Load()
	msg := "播放器已退出"
	if err != nil {
		msg += "：" + err.Error()
	}
	p.app.post(func() {
		if p.session.Load() != gen || !p.app.videoPlayerOpen {
			return
		}
		if played {
			p.app.notice = msg
			p.app.closePlayer()
			return
		}
		p.app.embedFailed(fmt.Errorf("%s", msg))
	})
}

// Close stops mpv, closes the pipe and destroys the child window. It is safe
// to call from the main thread more than once.
func (p *mpvEmbedder) Close() {
	if p.closed.Swap(true) {
		return
	}
	p.session.Add(1)
	p.send("quit")
	p.writeMu.Lock()
	pipe := p.pipe
	p.pipe = nil
	p.writeMu.Unlock()
	if pipe != nil {
		_ = pipe.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		cmd := p.cmd
		time.AfterFunc(2*time.Second, func() { _ = cmd.Process.Kill() })
	}
	p.destroyWindow()
	p.cmd = nil
}

// embedFailed is the app's side of a failed host: tear the player down and
// let the next frame open the page in the WebView fallback.
func (a *App) embedFailed(cause error) {
	if !a.videoPlayerOpen {
		return
	}
	a.videoPlayerOpen = false
	a.videoBusy = false
	if a.videoEmbed != nil {
		a.videoEmbed.Close()
	}
	if a.videoPageURL != "" {
		a.videoPending = a.videoPageURL
	}
	a.notice = "内置播放器没能播放，改用应用内网页播放：" + cause.Error()
}
