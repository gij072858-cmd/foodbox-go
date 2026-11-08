//go:build windows

package app

import (
	"strings"
	"testing"
	"time"
)

// The UI thread only enqueues commands; a full queue (mpv not draining) must
// drop commands rather than block the frame.
func TestMPVSendNeverBlocksWhenQueueFull(t *testing.T) {
	p := &mpvEmbedder{
		sess: &mpvSession{out: make(chan []byte, 1), stop: make(chan struct{})},
	}
	p.send("observe_property", 1, "pause") // fills the queue

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			p.send("seek", float64(i), "absolute")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("send blocked with a full command queue")
	}
}

// The progress bar needs an absolute seek, not a relative one.
func TestMPVSeekToIsAbsolute(t *testing.T) {
	p := &mpvEmbedder{
		sess: &mpvSession{out: make(chan []byte, 4), stop: make(chan struct{})},
	}
	p.SeekTo(12.5)
	select {
	case b := <-p.sess.out:
		got := string(b)
		for _, want := range []string{`"seek"`, "12.5", `"absolute"`} {
			if !strings.Contains(got, want) {
				t.Errorf("SeekTo command %q lacks %q", got, want)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("SeekTo sent no command")
	}
}

// Close must return at once even when no process or pipe was ever made, and
// stay safe to call again.
func TestMPVCloseReturnsQuickly(t *testing.T) {
	p := &mpvEmbedder{}
	start := time.Now()
	p.Close()
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("Close took %v, want it not to wait on anything", d)
	}
	p.Close()
}
