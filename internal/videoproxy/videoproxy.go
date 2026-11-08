// Package videoproxy resolves a direct media URL for a video page and hands
// it to an external player, so that watching a dish's tutorial opens a player
// window instead of a site page that wants a login.
//
// The resolution follows the endpoints iawia002/lux uses for Bilibili
// (extractors/bilibili/bilibili.go): x/player/playurl with fnval=16 for DASH,
// qn=127 for the best the account may have, and a Referer that every request
// must carry, the API's and the CDN's alike. DownBit — the MyGo download
// manager by MyGo's own author, which this approach is modelled on — does the
// same thing: sniff the page, resolve a direct URL, hand it off. Its source
// is private, so the endpoints come from lux, which reproduces them.
//
// Nothing here imports ui: it is plain net/http, testable with httptest, the
// way internal/cookmatch and internal/services are.
package videoproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Bilibili's web endpoints and the Referer they all demand. They are vars, not
// consts, so a test can point them at a canned server; nothing else sets them.
const (
	biliReferer = "https://www.bilibili.com"
	// A real browser identity matters: a made-up agent trips the site's risk
	// control long before a signed request should fail.
	biliUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

var (
	biliView    = "https://api.bilibili.com/x/web-interface/view"
	biliPlayURL = "https://api.bilibili.com/x/player/wbi/playurl"
	biliNav     = "https://api.bilibili.com/x/web-interface/nav"
	biliSPI     = "https://api.bilibili.com/x/frontend/finger/spi"

	// The retry knobs are vars so the signed-request tests can make them
	// instantaneous.
	biliAttempts   = 3
	biliRetryDelay = 300 * time.Millisecond
	biliHTTP       = &http.Client{Timeout: 20 * time.Second}
)

// Stream is one playable thing: the video track, its audio track when the
// site splits them (Bilibili's DASH always does), and the headers the CDN
// insists on.
type Stream struct {
	VideoURL string
	AudioURL string
	// VideoURLs and AudioURLs list the CDN candidates, primary first: the
	// player retries the next one when a host drops the stream.
	VideoURLs []string
	AudioURLs []string
	Headers   map[string]string
	Duration  time.Duration
	Title     string
	// Qndesc is what the site calls this quality, e.g. "1080P 高清".
	Qndesc string
}

// Referrer reports the Referer header, kept as a method so a caller cannot
// play a stream without passing it on.
func (s Stream) Referrer() string { return s.Headers["Referer"] }

// Supported reports whether this app can resolve a direct URL for pageURL.
// It answers false for anything it does not know — Xiaohongshu among them,
// which has no playurl equivalent — and the caller then shows the page in
// the in-app web view rather than pretending.
func Supported(pageURL string) bool {
	if _, ok := biliBVID(pageURL); ok {
		return true
	}
	return biliShortPattern.MatchString(pageURL)
}

// Platform names what a page's host is, for the settings copy and the button.
func Platform(pageURL string) string {
	if Supported(pageURL) {
		return "哔哩哔哩"
	}
	return ""
}

// Resolve turns a video page into a direct stream. sessdata is the login
// cookie; without it Bilibili answers with the qualities a stranger may see.
func Resolve(ctx context.Context, pageURL, sessdata string) (*Stream, error) {
	if bvid, ok := biliBVID(pageURL); ok {
		return resolveBilibili(ctx, bvid, biliPage(pageURL), sessdata)
	}
	if biliShortPattern.MatchString(pageURL) {
		if bvid, page, ok := shortBVID(ctx, pageURL); ok {
			return resolveBilibili(ctx, bvid, page, sessdata)
		}
	}
	return nil, fmt.Errorf("这个站点的视频只能在应用内网页里播放")
}

// biliPage reads ?p= from a video page. Zero means the page did not name one,
// and the first part is used.
func biliPage(pageURL string) int {
	u, err := url.Parse(pageURL)
	if err != nil {
		return 0
	}
	p, _ := strconv.Atoi(u.Query().Get("p"))
	if p < 1 {
		return 0
	}
	return p
}

// biliBVID pulls the bvid out of any of the shapes a shared link comes in.
var biliBVIDPattern = regexp.MustCompile(`(?i)\b(BV[0-9A-Za-z]{10})\b`)

// biliShortPattern is the shortener the mobile app copies: a shared link is
// b23.tv/xxxx, with no bvid in it until it redirects.
var biliShortPattern = regexp.MustCompile(`(?i)^https?://(?:b23\.tv|bili2233\.cn)/`)

func biliBVID(pageURL string) (string, bool) {
	m := biliBVIDPattern.FindString(pageURL)
	if m == "" {
		return "", false
	}
	// A bvid is case-sensitive end to end: upper-casing any of it turns
	// BV1Uv411N7Kh into a different, non-existent id and the site answers
	// -404. The match is exactly BV + ten characters, so it goes as it came.
	return m, len(m) == 12
}

// shortBVID follows a short link's redirects and reads the bvid off the
// landing address, or out of the page it lands on. lux resolves b23.tv the
// same way, one hop at a time, so a relative Location is resolved against the
// address that answered.
func shortBVID(ctx context.Context, pageURL string) (string, int, bool) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	next := pageURL
	for hop := 0; hop < 5; hop++ {
		if bvid, ok := biliBVID(next); ok {
			return bvid, biliPage(next), true
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return "", 0, false
		}
		req.Header.Set("Referer", biliReferer)
		req.Header.Set("User-Agent", biliUserAgent)
		resp, err := client.Do(req)
		if err != nil {
			return "", 0, false
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
		_ = resp.Body.Close()
		if loc := resp.Header.Get("Location"); loc != "" {
			to, err := req.URL.Parse(loc)
			if err != nil {
				return "", 0, false
			}
			next = to.String()
			continue
		}
		if bvid, ok := biliBVID(string(body)); ok {
			return bvid, biliPage(next), true
		}
		return "", 0, false
	}
	return "", 0, false
}

// viewReply is the slice of x/web-interface/view this app reads.
type viewReply struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Title    string `json:"title"`
		Duration int    `json:"duration"`
		CID      int64  `json:"cid"`
		Pages    []struct {
			CID int64 `json:"cid"`
		} `json:"pages"`
	} `json:"data"`
}

// playReply is the slice of x/player/playurl this app reads. DASH splits the
// picture and the sound into two lists; each entry is addressed by a baseUrl
// the CDN will only serve with the right Referer.
type playReply struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Duration int    `json:"timelength"`
		Dash     *dash  `json:"dash"`
		DURL     []durl `json:"durl"`
	} `json:"data"`
}

type dash struct {
	Video []media `json:"video"`
	Audio []media `json:"audio"`
}

type media struct {
	ID         int     `json:"id"`
	Codecid    int     `json:"codecid"`
	BaseURLs   urlList `json:"baseUrl"`
	BackupURLs urlList `json:"backupUrl"`
	Bandwidth  int     `json:"bandwidth"`
	Width      int     `json:"width"`
}

// urlList is one address or a list of them. The site answers baseUrl as a
// single string today and answered an array when this code was first written;
// both shapes have to parse, or every stream fails at the JSON boundary.
type urlList []string

func (u *urlList) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var one string
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*u = urlList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*u = many
	return nil
}

type durl struct {
	URL       string `json:"url"`
	Quality   int    `json:"quality"`
	TotalSize int    `json:"total_size"`
}

func resolveBilibili(ctx context.Context, bvid string, page int, sessdata string) (*Stream, error) {
	creds := fetchWBI(ctx, sessdata)
	referer := biliReferer + "/video/" + bvid
	var view viewReply
	if err := biliGet(ctx, biliView+"?bvid="+url.QueryEscape(bvid), sessdata, referer, creds, &view, nil); err != nil {
		return nil, err
	}

	// ?p=N chooses one part; the top-level cid is part 1. A page number
	// outside the list falls back to the first part instead of failing.
	cid := view.Data.CID
	if page > 0 && page <= len(view.Data.Pages) {
		cid = view.Data.Pages[page-1].CID
	}
	if cid == 0 && len(view.Data.Pages) > 0 {
		cid = view.Data.Pages[0].CID
	}
	if cid == 0 {
		return nil, fmt.Errorf("看不出这一页要播哪一段（%s）", biliMessage(view.Message))
	}

	stream := &Stream{
		Title:    view.Data.Title,
		Duration: time.Duration(view.Data.Duration) * time.Second,
		Headers: map[string]string{
			"Referer":    biliReferer,
			"User-Agent": biliUserAgent,
			"Origin":     biliReferer,
		},
	}

	// fnval=16 asks for DASH; qn=127 is the highest the account may have, and
	// fourk=1 lets 4K into the answer. Exactly what lux asks for.
	dashParams := url.Values{}
	dashParams.Set("bvid", bvid)
	dashParams.Set("cid", strconv.FormatInt(cid, 10))
	dashParams.Set("qn", "127")
	dashParams.Set("fnval", "16")
	dashParams.Set("fnver", "0")
	dashParams.Set("fourk", "1")
	dashParams.Set("otype", "json")
	var play playReply
	if err := biliGetSigned(ctx, biliPlayURL, dashParams, sessdata, bvid, &play); err != nil {
		return nil, err
	}
	if !streamFromPlay(&play, stream) {
		// Some videos answer a DASH request without a DASH object. Ask for
		// the legacy single-file form before giving up.
		legacyParams := url.Values{}
		legacyParams.Set("bvid", bvid)
		legacyParams.Set("cid", strconv.FormatInt(cid, 10))
		legacyParams.Set("qn", "64")
		legacyParams.Set("fnval", "0")
		legacyParams.Set("fnver", "0")
		legacyParams.Set("fourk", "0")
		legacyParams.Set("otype", "json")
		var legacy playReply
		if err := biliGetSigned(ctx, biliPlayURL, legacyParams, sessdata, bvid, &legacy); err != nil {
			return nil, err
		}
		if !streamFromPlay(&legacy, stream) {
			return nil, fmt.Errorf("站点没有给出可播的流")
		}
		play = legacy
	}
	if stream.VideoURL == "" {
		return nil, fmt.Errorf("站点给出的流地址是空的")
	}
	if play.Data.Duration > 0 {
		stream.Duration = time.Duration(play.Data.Duration) * time.Millisecond
	}
	return stream, nil
}

// streamFromPlay fills the stream from one playurl answer. It reports whether
// the answer carried a playable video at all.
func streamFromPlay(play *playReply, stream *Stream) bool {
	if play.Data.Dash != nil && len(play.Data.Dash.Video) > 0 {
		video, _ := pickBest(play.Data.Dash.Video, func(m media) int { return m.Bandwidth })
		videoURLs := uniqueURLs(video.BaseURLs, video.BackupURLs)
		if len(videoURLs) == 0 {
			return false
		}
		stream.VideoURL = videoURLs[0]
		stream.VideoURLs = videoURLs
		stream.Qndesc = qualityName(video.ID, video.Width)
		// DASH has no sound in the video track: the audio is a second file,
		// and a player must be told about both. mpv takes --audio-file.
		if play.Data.Dash.Audio != nil && len(play.Data.Dash.Audio) > 0 {
			audio, _ := pickBest(play.Data.Dash.Audio, func(m media) int { return m.Bandwidth })
			audioURLs := uniqueURLs(audio.BaseURLs, audio.BackupURLs)
			if len(audioURLs) > 0 {
				stream.AudioURL = audioURLs[0]
				stream.AudioURLs = audioURLs
			}
		}
		return true
	}
	if len(play.Data.DURL) > 0 {
		// The legacy single-file answer: picture and sound together, and the
		// only shape a player without a second input can play.
		stream.VideoURL = play.Data.DURL[0].URL
		stream.VideoURLs = []string{play.Data.DURL[0].URL}
		stream.Qndesc = strconv.Itoa(play.Data.DURL[0].Quality)
		return stream.VideoURL != ""
	}
	return false
}

// uniqueURLs flattens base and backup lists, keeping the first spelling of
// each address. The primary is always the first the site offered.
func uniqueURLs(lists ...urlList) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range lists {
		for _, u := range list {
			if u == "" || seen[u] {
				continue
			}
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// httpStatusError is a non-200 answer, kept separate so retry can tell a
// transient 5xx from a permanent 404.
type httpStatusError struct{ Code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("站点返回了 %d", e.Code) }

// apiError is the site's own {"code":..., "message":...} refusal.
type apiError struct {
	Code    int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("站点拒绝了请求（%s）", biliMessage(e.Message))
}

// biliGet asks an unsigned API (view, nav, fingerprint) with the headers and
// cookies Bilibili checks, retrying transient failures. allowed lets nav
// answer its -101 "not logged in" while still carrying the WBI keys.
func biliGet(ctx context.Context, endpoint, sessdata, referer string, creds wbiCreds, out any, allowed func(int) bool) error {
	var last error
	for attempt := 1; attempt <= biliAttempts; attempt++ {
		err := biliGetOnce(ctx, endpoint, sessdata, referer, creds, out)
		if err == nil {
			return nil
		}
		var ae *apiError
		if errors.As(err, &ae) && allowed != nil && allowed(ae.Code) {
			return nil
		}
		last = err
		if !biliRetryable(err) || attempt >= biliAttempts {
			break
		}
		if err := sleepCtx(ctx, time.Duration(attempt)*biliRetryDelay); err != nil {
			return err
		}
	}
	return last
}

// biliGetSigned signs params with the cached WBI keys, asks the endpoint and
// retries; a risk-control code drops the key cache so the next attempt signs
// with fresh keys.
func biliGetSigned(ctx context.Context, endpoint string, params url.Values, sessdata, bvid string, out any) error {
	referer := biliReferer
	if bvid != "" {
		referer = biliReferer + "/video/" + bvid
	}
	var last error
	for attempt := 1; attempt <= biliAttempts; attempt++ {
		creds := fetchWBI(ctx, sessdata)
		q := signedQuery(params, creds, wbiNow())
		err := biliGetOnce(ctx, endpoint+"?"+q, sessdata, referer, creds, out)
		if err == nil {
			return nil
		}
		var ae *apiError
		if errors.As(err, &ae) && wbiRiskCode(ae.Code) {
			resetWBICache()
		}
		last = err
		if !biliRetryable(err) || attempt >= biliAttempts {
			break
		}
		if err := sleepCtx(ctx, time.Duration(attempt)*biliRetryDelay); err != nil {
			return err
		}
	}
	return last
}

// biliGetOnce performs one request and decodes the JSON both into out and
// into the site's code envelope, so a refusal is not mistaken for a stream.
func biliGetOnce(ctx context.Context, endpoint, sessdata, referer string, creds wbiCreds, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", referer)
	req.Header.Set("User-Agent", biliUserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Origin", biliReferer)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	cookies := make([]string, 0, 3)
	if sessdata != "" {
		cookies = append(cookies, "SESSDATA="+sessdata)
	}
	if creds.buvid3 != "" {
		cookies = append(cookies, "buvid3="+creds.buvid3)
	}
	if creds.buvid4 != "" {
		cookies = append(cookies, "buvid4="+creds.buvid4)
	}
	if len(cookies) > 0 {
		req.Header.Set("Cookie", strings.Join(cookies, "; "))
	}
	resp, err := biliHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{Code: resp.StatusCode}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("看不懂站点的回答：%w", err)
	}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Code != 0 {
		return &apiError{Code: env.Code, Message: env.Message}
	}
	return nil
}

func biliRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.Code == 412 || se.Code == 429 || se.Code >= 500
	}
	var ae *apiError
	if errors.As(err, &ae) {
		return wbiRiskCode(ae.Code)
	}
	var ue *url.Error
	return errors.As(err, &ue)
}

// wbiRiskCode lists the refusals a fresh WBI key or a short backoff can fix.
func wbiRiskCode(code int) bool {
	switch code {
	case -352, -403, -412, -799:
		return true
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// pickBest takes the entry with the most bandwidth, which is the quality qn
// asked for; lux sorts by bandwidth rather than trusting the site's order.
func pickBest[T any](items []T, bandwidth func(T) int) (T, bool) {
	var best T
	if len(items) == 0 {
		return best, false
	}
	best, top := items[0], bandwidth(items[0])
	for _, item := range items[1:] {
		if b := bandwidth(item); b > top {
			best, top = item, b
		}
	}
	return best, true
}

func biliMessage(m string) string {
	if m == "" {
		return "未知原因"
	}
	return m
}

// qualityName says what the picture is, in the words the site itself uses for
// the qn values: 127 is whatever the account may have, 112 is 1080P 加高清,
// 80 是 1080P, 64 是 720P, 32 是 480P, 16 是 720P 投屏.
func qualityName(id, width int) string {
	switch id {
	case 127:
		return "账号可用的最高"
	case 116:
		return "高帧率 4K"
	case 112:
		return "1080P 加高清"
	case 80:
		return "1080P"
	case 74:
		return "骁龙 720P"
	case 64:
		return "720P"
	case 32:
		return "480P"
	case 16:
		return "720P 投屏"
	}
	if width > 0 {
		return fmt.Sprintf("%d 宽", width)
	}
	return "默认"
}
