//go:build !windows

package app

import (
	"errors"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/videoproxy"
)

// unsupportedEmbedder keeps the non-Windows build compiling and sends the
// app down its external-player/web fallback.
type unsupportedEmbedder struct{}

func newVideoEmbedder(*App) videoEmbedder { return unsupportedEmbedder{} }

func (unsupportedEmbedder) Supported() bool { return false }

func (unsupportedEmbedder) Open(uintptr, videoproxy.Found, *videoproxy.Stream, ui.Rect) error {
	return errors.New("this platform has no embedded video host")
}

func (unsupportedEmbedder) Resize(ui.Rect) {}
func (unsupportedEmbedder) SetPaused(bool) {}
func (unsupportedEmbedder) Seek(float64)   {}
func (unsupportedEmbedder) Close()         {}
