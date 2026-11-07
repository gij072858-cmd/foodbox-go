package videoproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// serve answers the two Bilibili APIs from a canned pair of replies, and
// records the headers it was asked with — the Referer is the whole point of
// the exercise, since a CDN that wants none would not be worth testing.
func serve(t *testing.T, view, play map[string]any, got *map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		switch {
		case strings.HasPrefix(r.URL.Path, "/x/web-interface/view"):
			body = view
		case strings.HasPrefix(r.URL.Path, "/x/player/playurl"):
			if r.URL.Query().Get("fnval") != "16" {
				t.Errorf("playurl asked without fnval=16: %q", r.URL.RawQuery)
			}
			if r.URL.Query().Get("qn") != "127" {
				t.Errorf("playurl asked without the best quality: %q", r.URL.RawQuery)
			}
			body = play
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got != nil {
			(*got)[r.URL.Path] = r.Header.Get("Referer") + "|" + r.Header.Get("Cookie")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func biliURLs(t *testing.T, s *httptest.Server) {
	t.Helper()
	oldView, oldPlay := biliView, biliPlayURL
	biliView = s.URL + "/x/web-interface/view"
	biliPlayURL = s.URL + "/x/player/playurl"
	t.Cleanup(func() { biliView, biliPlayURL = oldView, oldPlay })
}

const testPage = "https://www.bilibili.com/video/BV1GJ411x7h7/"

func TestResolveDASHSplitsPictureAndSound(t *testing.T) {
	headers := map[string]string{}
	view := map[string]any{"code": 0, "data": map[string]any{
		"title": "青椒肉丝怎么做", "duration": 125, "cid": 4242,
	}}
	play := map[string]any{"code": 0, "data": map[string]any{
		"timelength": 125000,
		"dash": map[string]any{
			"video": []any{
				map[string]any{"id": 64, "codecid": 7, "bandwidth": 900000, "width": 1280,
					"baseUrl": []string{"https://cdn.example/v-weak.m4s"}},
				map[string]any{"id": 80, "codecid": 12, "bandwidth": 3000000, "width": 1920,
					"baseUrl": "https://cdn.example/v-best.m4s"},
			},
			"audio": []any{
				map[string]any{"id": 30280, "bandwidth": 192000,
					"baseUrl": []string{"https://cdn.example/a-best.m4s"}},
			},
		},
	}}
	srv := serve(t, view, play, &headers)
	defer srv.Close()
	biliURLs(t, srv)

	stream, err := Resolve(context.Background(), testPage, "sess-abc")
	if err != nil {
		t.Fatal(err)
	}
	// The widest picture, not the first one listed.
	if stream.VideoURL != "https://cdn.example/v-best.m4s" {
		t.Errorf("video = %q, want the highest bandwidth one", stream.VideoURL)
	}
	if stream.AudioURL != "https://cdn.example/a-best.m4s" {
		t.Errorf("audio = %q, want the DASH audio track", stream.AudioURL)
	}
	if stream.Title != "青椒肉丝怎么做" || stream.Duration.Seconds() != 125 {
		t.Errorf("title/duration = %q / %v", stream.Title, stream.Duration)
	}
	// Every request must have carried the Referer, and the login cookie when
	// the caller has one.
	for path, seen := range headers {
		if !strings.HasPrefix(seen, "https://www.bilibili.com|") {
			t.Errorf("%s got Referer %q", path, seen)
		}
		if !strings.HasSuffix(seen, "|SESSDATA=sess-abc") {
			t.Errorf("%s got Cookie %q", path, seen)
		}
	}

	// mpv gets both tracks and the header on the wire; VLC would play the
	// picture in silence, so the pair is refused to it.
	mpv := LaunchArgs(PlayerMPV, "mpv.exe", *stream)
	joined := strings.Join(mpv, " ")
	if !strings.Contains(joined, "--audio-file=https://cdn.example/a-best.m4s") {
		t.Errorf("mpv argv has no second track: %q", joined)
	}
	// mpv splits the list on commas; the User-Agent itself holds a semicolon,
	// which is why a semicolon cannot be the separator.
	fields := ""
	for _, arg := range mpv {
		if strings.HasPrefix(arg, "--http-header-fields=") {
			fields = strings.TrimPrefix(arg, "--http-header-fields=")
		}
	}
	if !strings.Contains(fields, ",") ||
		!strings.Contains(fields, "Referer: https://www.bilibili.com") ||
		!strings.Contains(fields, "User-Agent: ") {
		t.Errorf("mpv header fields = %q, want both headers comma separated", fields)
	}
	if !strings.HasSuffix(joined, "https://cdn.example/v-best.m4s") {
		t.Errorf("mpv argv does not end on the video: %q", joined)
	}
	vlc := strings.Join(LaunchArgs(PlayerVLC, "vlc.exe", *stream), " ")
	if !strings.Contains(vlc, "--http-referrer=https://www.bilibili.com") {
		t.Errorf("vlc argv has no referrer: %q", vlc)
	}
	if _, err = Launch(context.Background(), Found{Path: "vlc.exe", Kind: PlayerVLC}, *stream); err == nil {
		t.Error("a split DASH pair was handed to VLC, which has no second audio input")
	}
}

func TestResolveFallsBackToSingleFile(t *testing.T) {
	view := map[string]any{"code": 0, "data": map[string]any{"cid": 7, "title": "x", "duration": 10}}
	play := map[string]any{"code": 0, "data": map[string]any{
		"timelength": 10000,
		"durl":       []any{map[string]any{"url": "https://cdn.example/one.mp4", "quality": 80}},
	}}
	srv := serve(t, view, play, nil)
	defer srv.Close()
	biliURLs(t, srv)

	stream, err := Resolve(context.Background(), testPage, "")
	if err != nil {
		t.Fatal(err)
	}
	if stream.VideoURL != "https://cdn.example/one.mp4" || stream.AudioURL != "" {
		t.Errorf("durl path gave %+v", stream)
	}
	if _, err = Launch(context.Background(), Found{Path: "vlc.exe", Kind: PlayerVLC}, *stream); err != nil {
		// A single file is exactly what VLC can take; the exec will fail in
		// the test, but the argument check must not.
		if strings.Contains(err.Error(), "DASH") {
			t.Errorf("a progressive file was refused for VLC: %v", err)
		}
	}
}

func TestResolveFollowsShortLink(t *testing.T) {
	// The mobile app copies b23.tv links; they hold no bvid until they
	// redirect, so the resolver has to take one hop before it can ask.
	oldPattern := biliShortPattern
	if !Supported("https://b23.tv/abc123") {
		t.Error("a b23.tv share link is not recognised as Bilibili")
	}
	biliShortPattern = regexp.MustCompile(`^http://127\.0\.0\.1:`)
	t.Cleanup(func() { biliShortPattern = oldPattern })

	view := map[string]any{"code": 0, "data": map[string]any{"cid": 7, "title": "短链", "duration": 10}}
	play := map[string]any{"code": 0, "data": map[string]any{
		"timelength": 10000,
		"durl":       []any{map[string]any{"url": "https://cdn.example/one.mp4", "quality": 80}},
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("/s/abc123", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/video/BV1GJ411x7h7/", http.StatusFound)
	})
	mux.HandleFunc("/x/web-interface/view", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(view)
	})
	mux.HandleFunc("/x/player/playurl", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(play)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	biliURLs(t, srv)

	stream, err := Resolve(context.Background(), srv.URL+"/s/abc123", "")
	if err != nil {
		t.Fatal(err)
	}
	if stream.Title != "短链" {
		t.Errorf("title = %q, want the one behind the redirect", stream.Title)
	}
}

func TestBVIDKeepsItsCase(t *testing.T) {
	// A bvid is case-sensitive; upper-casing part of it used to turn
	// BV1Uv411N7Kh into BV1UV411N7Kh, which the site answers with -404.
	for _, c := range []struct{ in, want string }{
		{"https://www.bilibili.com/video/BV1Uv411N7Kh", "BV1Uv411N7Kh"},
		{"【搬运】https://www.bilibili.com/video/BV1GJ411x7h7/?p=2 分享自哔哩哔哩", "BV1GJ411x7h7"},
	} {
		got, ok := biliBVID(c.in)
		if !ok || got != c.want {
			t.Errorf("biliBVID(%q) = %q, %v; want %q", c.in, got, ok, c.want)
		}
	}
}

func TestResolveRefusesWhatItCannotKnow(t *testing.T) {
	if Supported("https://www.xiaohongshu.com/explore/abc123") {
		t.Error("Xiaohongshu claims a playurl endpoint; it has none")
	}
	if Platform(testPage) != "哔哩哔哩" {
		t.Errorf("Platform(%q) = %q", testPage, Platform(testPage))
	}
	if _, err := Resolve(context.Background(), "https://www.xiaohongshu.com/explore/abc", ""); err == nil {
		t.Error("Resolve answered for a site it does not know")
	}

	// A site that says no (a -403 for missing WBI signing among them) must
	// come back as its own reason, not as a silent empty stream.
	view := map[string]any{"code": 0, "data": map[string]any{"cid": 7}}
	play := map[string]any{"code": -403, "message": "请求过于频繁"}
	srv := serve(t, view, play, nil)
	defer srv.Close()
	biliURLs(t, srv)
	if _, err := Resolve(context.Background(), testPage, ""); err == nil ||
		!strings.Contains(err.Error(), "请求过于频繁") {
		t.Errorf("the site's own reason was lost: %v", err)
	}
}

func TestFindPlayerPrefersMPV(t *testing.T) {
	dir := t.TempDir()
	// Nothing installed yet: the app's own portable slot is empty.
	if got := FindPlayer(dir); got.Kind != PlayerNone {
		t.Errorf("found %v with an empty machine", got)
	}
	// The portable slot the app would fill is looked for first, and it knows
	// the copy came from itself.
	bin := portablePath(dir)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	// An executable by that name is enough for the probe; it is never run.
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := FindPlayer(dir)
	if got.Kind != PlayerMPV || !got.Portable || got.Path != bin {
		t.Errorf("FindPlayer = %+v, want the app's own portable mpv at %s", got, bin)
	}
	if len(LaunchArgs(PlayerNone, "", Stream{})) != 0 {
		t.Error("PlayerNone should give no command line at all")
	}
}
