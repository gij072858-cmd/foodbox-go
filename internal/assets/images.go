package assets

import (
	"container/list"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Images is the app's single image cache. Decoding happens on a bounded
// number of goroutines with MyGo's decoder (PNG, JPEG, GIF, WebP, BMP and
// EXIF orientation); the resulting ui.Bitmap is handed to the view by Get.
// MyGo has no decoded-bitmap cache, so the LRU is the app's.
//
// The LRU keeps at most Max decoded bitmaps. Request is cheap to call from
// the view: a decoded, pending or failed key returns at once.
type Images struct {
	mu      sync.Mutex
	max     int
	order   *list.List
	entries map[string]*list.Element
	ready   map[string]*ui.Bitmap
	pending map[string]bool
	failed  map[string]bool
	sem     chan struct{}
}

type imageEntry struct {
	key string
	bm  *ui.Bitmap
	// shown marks a bitmap that has had a frame to reach the screen. The
	// next Get drops its CPU pixels: a GPU renderer keeps the textures it
	// uploaded, and re-decodes from the source bytes if it ever needs the
	// pixels again (ui/bitmap.go ReleasePixels).
	shown    bool
	released bool
}

// NewImages makes a cache holding at most max decoded bitmaps and decoding
// at most four at once.
func NewImages(max int) *Images {
	if max < 8 {
		max = 8
	}
	return &Images{
		max:     max,
		order:   list.New(),
		entries: make(map[string]*list.Element, max),
		ready:   make(map[string]*ui.Bitmap),
		pending: make(map[string]bool),
		failed:  make(map[string]bool),
		sem:     make(chan struct{}, 4),
	}
}

// Get returns the bitmap for an image key, or nil while it decodes or when
// it failed. Call it from the view (main thread).
func (s *Images) Get(key string) *ui.Bitmap {
	if key == "" {
		return nil
	}
	s.mu.Lock()
	if el, ok := s.entries[key]; ok {
		s.order.MoveToFront(el)
		entry := el.Value.(*imageEntry)
		bm := entry.bm
		if entry.shown && !entry.released {
			entry.released = true
			bm.ReleasePixels()
		}
		entry.shown = true
		s.mu.Unlock()
		return bm
	}
	bm, ok := s.ready[key]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.ready, key)
	el := s.order.PushFront(&imageEntry{key: key, bm: bm})
	s.entries[key] = el
	for s.order.Len() > s.max {
		old := s.order.Back()
		if old == nil {
			break
		}
		entry := old.Value.(*imageEntry)
		entry.bm.ReleasePixels()
		delete(s.entries, entry.key)
		s.order.Remove(old)
	}
	s.mu.Unlock()
	return bm
}

// Request starts decoding an image key if it is not cached, pending or
// known to fail. notify (optional) is called after a successful decode,
// usually posting a redraw to the main thread.
func (s *Images) Request(key string, notify func()) {
	if key == "" {
		return
	}
	s.mu.Lock()
	if _, ok := s.entries[key]; ok {
		s.mu.Unlock()
		return
	}
	if _, ok := s.ready[key]; ok {
		s.mu.Unlock()
		return
	}
	if s.pending[key] || s.failed[key] {
		s.mu.Unlock()
		return
	}
	s.pending[key] = true
	s.mu.Unlock()

	go func() {
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		data, err := Image(key)
		var bm *ui.Bitmap
		if err == nil {
			bm, err = ui.DecodeBitmap(data)
		}
		s.mu.Lock()
		delete(s.pending, key)
		if err != nil {
			s.failed[key] = true
		} else {
			s.ready[key] = bm
		}
		s.mu.Unlock()
		if err == nil && notify != nil {
			notify()
		}
	}()
}

// Forget drops a failed key, so a later request retries it (used after
// assets change on disk in development).
func (s *Images) Forget(key string) {
	s.mu.Lock()
	delete(s.failed, key)
	s.mu.Unlock()
}

// Cached returns how many bitmaps the LRU holds, for tests.
func (s *Images) Cached() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order.Len()
}
