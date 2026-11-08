package videoproxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// A Player is an external program that can be handed a resolved stream.
type Player int

const (
	// PlayerNone is what FindPlayer answers when there is nothing on this
	// machine to play with; the caller then shows the page in the web view.
	PlayerNone Player = iota
	PlayerMPV
	PlayerVLC
)

func (p Player) String() string {
	switch p {
	case PlayerMPV:
		return "mpv"
	case PlayerVLC:
		return "VLC"
	}
	return "无"
}

// Found is a player located on this machine.
type Found struct {
	Path string
	Kind Player
	// Portable says the app put this one there itself, so the settings page
	// can offer to remove it.
	Portable bool
}

// mpvOnly explains why a DASH stream needs mpv: Bilibili's answer splits the
// picture and the sound into two files, and only mpv is told both at once.
// VLC takes one URL and would play the picture in silence.
var errNeedMPV = errors.New("这一条是 DASH 分轨（画面与声音两个地址），只有 mpv 能同时播")

// ErrNoPlayer says nothing that could play was found.
var ErrNoPlayer = errors.New("本机没找到能播的播放器")

// FindPlayer looks for mpv or VLC: first the copy this app installed, then
// the usual install directories, then PATH. It answers the first thing that
// exists, preferring mpv, because a DASH stream cannot be played by anything
// else here.
func FindPlayer(appDir string) Found {
	for _, c := range candidates(appDir) {
		if abs, err := exec.LookPath(c); err == nil && abs != "" {
			return Found{Path: abs, Kind: kindOf(abs), Portable: c == portablePath(appDir)}
		}
		// LookPath only finds an executable; a directory holding a portable
		// build is checked by name instead.
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return Found{Path: c, Kind: kindOf(c), Portable: c == portablePath(appDir)}
		}
	}
	return Found{Kind: PlayerNone}
}

// candidates is where a player may be, in the order they are tried.
func candidates(appDir string) []string {
	if runtime.GOOS != "windows" {
		return []string{"mpv", "vlc"}
	}
	out := []string{portablePath(appDir)}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		out = append(out, filepath.Join(local, "Programs", "mpv", "mpv.exe"))
	}
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if root == "" {
			continue
		}
		out = append(out, filepath.Join(root, "mpv", "mpv.exe"),
			filepath.Join(root, "VideoLAN", "VLC", "vlc.exe"))
	}
	return append(out, "mpv", "vlc")
}

// portablePath is the copy this app would unpack into its own data directory.
func portablePath(appDir string) string {
	name := "mpv"
	if runtime.GOOS == "windows" {
		name = "mpv.exe"
	}
	return filepath.Join(appDir, "mpv-portable", name)
}

func kindOf(path string) Player {
	if strings.Contains(strings.ToLower(filepath.Base(path)), "vlc") {
		return PlayerVLC
	}
	return PlayerMPV
}

// LaunchArgs is the command line a player gets for one stream: the CDN's
// headers, and both DASH tracks when there are two. It is a separate function
// so the exact argv is something a test can read.
func LaunchArgs(player Player, exe string, s Stream) []string {
	headers := make([]string, 0, len(s.Headers))
	for k, v := range s.Headers {
		headers = append(headers, k+": "+v)
	}
	switch player {
	case PlayerVLC:
		// VLC's referrer option is the one that reaches its HTTP access
		// module; it has no second audio input, so a DASH pair cannot play.
		args := []string{exe, "--network-caching=1500",
			"--http-referrer=" + s.Referrer()}
		return append(args, s.VideoURL)
	case PlayerMPV:
		// mpv splits --http-header-fields on commas; a semicolon would fold
		// both headers into one malformed Referer and the CDN answers 403.
		args := []string{exe, "--force-window=yes", "--idle=no",
			"--title=" + s.Title,
			"--http-header-fields=" + strings.Join(headers, ",")}
		if s.AudioURL != "" {
			args = append(args, "--audio-file="+s.AudioURL)
		}
		return append(args, s.VideoURL)
	}
	return nil
}

// EmbeddedMPVArgs is the command line for mpv rendering into a child window
// the app owns, controlled over its JSON IPC pipe. --wid means mpv creates
// no window of its own, so the app supplies the frame and every control.
// --force-window stays off: with --wid a forced window would be a second,
// system-chromed one, which is exactly what this path removes.
func EmbeddedMPVArgs(exe string, hwnd uintptr, pipe string, s Stream) []string {
	headers := make([]string, 0, len(s.Headers))
	for k, v := range s.Headers {
		headers = append(headers, k+": "+v)
	}
	args := []string{
		exe,
		"--wid=" + strconv.FormatUint(uint64(hwnd), 10),
		"--input-ipc-server=" + pipe,
		"--no-border",
		"--force-window=no",
		"--osc=no",
		"--osd-level=0",
		"--keep-open=yes",
		"--idle=no",
		"--hwdec=auto-safe",
		"--really-quiet",
		"--http-header-fields=" + strings.Join(headers, ","),
	}
	if s.AudioURL != "" {
		args = append(args, "--audio-file="+s.AudioURL)
	}
	return append(args, s.VideoURL)
}

// Launch starts the player on a stream and returns the running command, so
// the app can wait on it and stop it when the dish page closes.
func Launch(ctx context.Context, found Found, s Stream) (*exec.Cmd, error) {
	if found.Kind == PlayerNone {
		return nil, ErrNoPlayer
	}
	if s.AudioURL != "" && found.Kind != PlayerMPV {
		return nil, errNeedMPV
	}
	args := LaunchArgs(found.Kind, found.Path, s)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	// A GUI player needs no pipes; keeping them closed means closing the app
	// cannot leave a process waiting on a writer.
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("播放器起不来：%w", err)
	}
	return cmd, nil
}
