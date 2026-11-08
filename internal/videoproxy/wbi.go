package videoproxy

// Bilibili's playurl endpoint rejects unsigned requests often enough to be
// useless in practice (the old resolver got about one success in ten). The
// signed form is WBI: the site publishes two keys through x/web-interface/nav,
// a fixed permutation mixes them into a 32-character secret, and every
// parameter is sorted, encoded, timestamped and hashed with that secret.
//
// The algorithm is the one bilibili-API-collect documents and every
// third-party client implements:
//
//	https://github.com/SocialSisterYi/bilibili-API-collect/blob/master/docs/misc/sign/wbi.md

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// mixinKeyEncTab is the site's fixed permutation of the concatenated keys.
var mixinKeyEncTab = [...]int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35,
	27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13,
	37, 48, 7, 16, 24, 55, 40, 61, 26, 17, 0, 1, 60, 51, 30, 4,
	22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11, 36, 20, 34, 44, 52,
}

// mixinKey turns the two published keys into the 32-character WBI secret.
func mixinKey(imgKey, subKey string) string {
	raw := imgKey + subKey
	if len(raw) == 0 {
		return ""
	}
	out := make([]byte, 0, 32)
	for _, i := range mixinKeyEncTab {
		if i >= len(raw) {
			continue
		}
		out = append(out, raw[i])
		if len(out) == 32 {
			break
		}
	}
	return string(out)
}

// keyFromURL is the filename without its extension, which is the key part of
// a wbi_img URL such as .../7cd084941338484aae1ad9425b84077c.png.
func keyFromURL(raw string) string {
	if i := strings.LastIndexByte(raw, '/'); i >= 0 {
		raw = raw[i+1:]
	}
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

// filterWBIValue removes the characters the site leaves out of the signed
// value. It is applied before encoding, as the reference implementations do.
func filterWBIValue(v string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '!', '\'', '(', ')', '*':
			return -1
		}
		return r
	}, v)
}

// signParams returns the sorted, encoded query with wts and w_rid appended.
// The encoding uses %20 for spaces, matching encodeURIComponent; the reference
// implementations hash the exact string they send.
func signParams(params url.Values, mixin string, now time.Time) string {
	q := url.Values{}
	for k, values := range params {
		if k == "w_rid" || k == "wts" {
			continue
		}
		for _, v := range values {
			q.Add(k, filterWBIValue(v))
		}
	}
	q.Set("wts", strconv.FormatInt(now.Unix(), 10))

	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteByte('=')
		b.WriteString(strings.ReplaceAll(url.QueryEscape(q.Get(k)), "+", "%20"))
	}
	query := b.String()
	sum := md5.Sum([]byte(query + mixin))
	return query + "&w_rid=" + hex.EncodeToString(sum[:])
}

// signedQuery signs when the keys are known and falls back to a plain query
// when nav is unreachable, so a transient key failure still has a chance.
func signedQuery(params url.Values, creds wbiCreds, now time.Time) string {
	if creds.mixin == "" {
		p := url.Values{}
		for k, v := range params {
			p[k] = append([]string(nil), v...)
		}
		return p.Encode()
	}
	return signParams(params, creds.mixin, now)
}

// wbiCreds is the secret and the anonymous device cookies Bilibili expects.
type wbiCreds struct {
	mixin  string
	buvid3 string
	buvid4 string
}

type wbiNavReply struct {
	Code int `json:"code"`
	Data struct {
		WbiImg struct {
			ImgURL string `json:"img_url"`
			SubURL string `json:"sub_url"`
		} `json:"wbi_img"`
	} `json:"data"`
}

type wbiSPIReply struct {
	Code int `json:"code"`
	Data struct {
		B3 string `json:"b_3"`
		B4 string `json:"b_4"`
	} `json:"data"`
}

var (
	wbiMu       sync.Mutex
	wbiCached   wbiCreds
	wbiCachedAt time.Time
	wbiCachedOK bool
	// wbiTTL refreshes the keys well before the site rotates them (daily).
	wbiTTL = 10 * time.Minute
	// wbiNow is a test hook, like the endpoint vars.
	wbiNow = time.Now
)

func resetWBICache() {
	wbiMu.Lock()
	wbiCached, wbiCachedAt, wbiCachedOK = wbiCreds{}, time.Time{}, false
	wbiMu.Unlock()
}

// fetchWBI returns the cached keys or asks nav and the fingerprint endpoint
// for them. A failure is not fatal: the caller still tries an unsigned
// request, and the retry loop refreshes on a risk-control answer.
func fetchWBI(ctx context.Context, sessdata string) wbiCreds {
	wbiMu.Lock()
	if wbiCachedOK && wbiNow().Sub(wbiCachedAt) < wbiTTL {
		c := wbiCached
		wbiMu.Unlock()
		return c
	}
	wbiMu.Unlock()

	var c wbiCreds
	var nav wbiNavReply
	// nav answers -101 when nobody is logged in, but still carries the keys.
	if err := biliGet(ctx, biliNav, sessdata, biliReferer, wbiCreds{}, &nav,
		func(int) bool { return true }); err == nil {
		img := keyFromURL(nav.Data.WbiImg.ImgURL)
		sub := keyFromURL(nav.Data.WbiImg.SubURL)
		if img != "" && sub != "" {
			c.mixin = mixinKey(img, sub)
		}
	}
	var spi wbiSPIReply
	if err := biliGet(ctx, biliSPI, sessdata, biliReferer, wbiCreds{}, &spi,
		func(int) bool { return true }); err == nil {
		c.buvid3, c.buvid4 = strings.TrimSpace(spi.Data.B3), strings.TrimSpace(spi.Data.B4)
	}
	if c.mixin != "" || c.buvid3 != "" {
		wbiMu.Lock()
		wbiCached, wbiCachedAt, wbiCachedOK = c, wbiNow(), true
		wbiMu.Unlock()
	}
	return c
}
