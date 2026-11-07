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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// Bilibili's web endpoints and the Referer they all demand. They are vars, not
// consts, so a test can point them at a canned server; nothing else sets them.
const (
	biliReferer   = "https://www.bilibili.com"
	biliUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) FoodBox/1.0"
)

var (
	biliView    = "https://api.bilibili.com/x/web-interface/view"
	biliPlayURL = "https://api.bilibili.com/x/player/playurl"
)

// Stream is one playable thing: the video track, its audio track when the
// site splits them (Bilibili's DASH always does), and the headers the CDN
// insists on.
type Stream struct {
	VideoURL string
	AudioURL string
	Headers  map[string]string
	Duration time.Duration
	Title    string
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
	bvid, ok := biliBVID(pageURL)
	if !ok && biliShortPattern.MatchString(pageURL) {
		bvid, ok = shortBVID(ctx, pageURL)
	}
	if ok {
		return resolveBilibili(ctx, bvid, sessdata)
	}
	return nil, fmt.Errorf("这个站点的视频只能在应用内网页里播放")
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
func shortBVID(ctx context.Context, pageURL string) (string, bool) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	next := pageURL
	for hop := 0; hop < 5; hop++ {
		if bvid, ok := biliBVID(next); ok {
			return bvid, true
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return "", false
		}
		req.Header.Set("Referer", biliReferer)
		req.Header.Set("User-Agent", biliUserAgent)
		resp, err := client.Do(req)
		if err != nil {
			return "", false
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
		_ = resp.Body.Close()
		if loc := resp.Header.Get("Location"); loc != "" {
			to, err := req.URL.Parse(loc)
			if err != nil {
				return "", false
			}
			next = to.String()
			continue
		}
		return biliBVID(string(body))
	}
	return "", false
}

// viewReply is the slice of x/web-interface/view this app reads.
type viewReply struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Title     string `json:"title"`
		Duration  int    `json:"duration"`
		CID       int64  `json:"cid"`
		MultiPart []struct {
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

func resolveBilibili(ctx context.Context, bvid, sessdata string) (*Stream, error) {
	var view viewReply
	if err := biliGet(ctx, biliView+"?bvid="+url.QueryEscape(bvid), sessdata, &view); err != nil {
		return nil, err
	}
	if view.Code != 0 || view.Data.CID == 0 {
		if len(view.Data.MultiPart) > 0 {
			view.Data.CID = view.Data.MultiPart[0].CID
		}
	}
	if view.Data.CID == 0 {
		return nil, fmt.Errorf("看不出这一页要播哪一段（%s）", biliMessage(view.Message))
	}

	// fnval=16 asks for DASH; qn=127 is the highest the account may have, and
	// fourk=1 lets 4K into the answer. Exactly what lux asks for.
	q := fmt.Sprintf("bvid=%s&cid=%d&qn=127&fnval=16&fnver=0&fourk=1&otype=json",
		url.QueryEscape(bvid), view.Data.CID)
	var play playReply
	if err := biliGet(ctx, biliPlayURL+"?"+q, sessdata, &play); err != nil {
		return nil, err
	}
	if play.Code != 0 {
		return nil, fmt.Errorf("站点拒绝了取直链的请求（%s）", biliMessage(play.Message))
	}

	stream := &Stream{
		Title:    view.Data.Title,
		Duration: time.Duration(play.Data.Duration) * time.Millisecond,
		Headers:  map[string]string{"Referer": biliReferer, "User-Agent": biliUserAgent},
	}
	switch {
	case play.Data.Dash != nil && len(play.Data.Dash.Video) > 0:
		video, _ := pickBest(play.Data.Dash.Video, func(m media) int { return m.Bandwidth })
		stream.VideoURL = firstURL(video.BaseURLs, video.BackupURLs)
		stream.Qndesc = qualityName(video.ID, video.Width)
		// DASH has no sound in the video track: the audio is a second file,
		// and a player must be told about both. mpv takes --audio-file.
		if play.Data.Dash.Audio != nil {
			audio, _ := pickBest(play.Data.Dash.Audio, func(m media) int { return m.Bandwidth })
			stream.AudioURL = firstURL(audio.BaseURLs, audio.BackupURLs)
		}
	case len(play.Data.DURL) > 0:
		// The legacy single-file answer: picture and sound together, and the
		// only shape a player without a second input can play.
		stream.VideoURL = play.Data.DURL[0].URL
		stream.Qndesc = strconv.Itoa(play.Data.DURL[0].Quality)
	default:
		return nil, fmt.Errorf("站点没有给出可播的流")
	}
	if stream.VideoURL == "" {
		return nil, fmt.Errorf("站点给出的流地址是空的")
	}
	return stream, nil
}

// biliGet asks an API with the headers Bilibili checks: a Referer from its own
// site, a browser's User-Agent, and the login cookie when there is one.
func biliGet(ctx context.Context, endpoint, sessdata string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", biliReferer)
	req.Header.Set("User-Agent", biliUserAgent)
	if sessdata != "" {
		req.Header.Set("Cookie", "SESSDATA="+sessdata)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("站点返回了 %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("看不懂站点的回答：%w", err)
	}
	return nil
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

func firstURL(lists ...urlList) string {
	for _, urls := range lists {
		if len(urls) > 0 {
			return urls[0]
		}
	}
	return ""
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
