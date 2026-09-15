package source

import (
	"net/url"
	"strings"
	"unicode"
)

// volatileParams are query keys that do not identify a post and only vary
// with list pagination / tracking. Stripping them canonicalizes the URL so
// the same post reached via different list pages dedups to one item.
var volatileParams = map[string]bool{
	"page":      true,
	"pageindex": true,
	"pg":        true,
	"fromlist":  true,
	"ref":       true,
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"fbclid":    true,
	"gclid":     true,
	"spm":       true,
}

// NormalizeURL strips the fragment and known volatile query parameters
// (pagination, tracking) while preserving the parameters that identify the
// post (e.g. dcinside's id/no). Original key order and value encoding are
// kept so the result stays a valid, clickable "본문 보기" link. Non-parseable
// input is returned unchanged.
func NormalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""

	// 루리웹 글 주소는 글 번호가 **경로**에 있고(/…/read/<번호>), 쿼리는
	// 「어느 목록에서 눌러 들어왔나」만 담는다(m=selection&t=now,
	// m=user_info, cate=…&view=gallery — 2026-09-15 베스트 목록 실측에서
	// read 링크의 쿼리 키는 m·t·cate·view 넷뿐이었다). 그래서 같은 글이
	// 목록 위치에 따라 다른 쿼리를 달고 나와 dedup 키가 갈라진다 — 실측
	// 414건 중 29건(7%)이 «?m=» 값만 달라 두 번 발송됐다.
	//
	// ⚠루리웹으로만 좁힌다. MLB파크는 m=view 가 고정이고 글 번호도 쿼리에
	//   있어서(b.php?m=view&b=…&id=…) 쿼리를 비우면 글을 구별할 수 없게 된다.
	if strings.EqualFold(u.Hostname(), "bbs.ruliweb.com") && strings.Contains(u.Path, "/read/") {
		u.RawQuery = ""
	}

	if u.RawQuery != "" {
		pairs := strings.Split(u.RawQuery, "&")
		kept := pairs[:0]
		for _, p := range pairs {
			key := p
			if i := strings.IndexByte(p, '='); i >= 0 {
				key = p[:i]
			}
			if volatileParams[strings.ToLower(key)] {
				continue
			}
			kept = append(kept, p)
		}
		u.RawQuery = strings.Join(kept, "&")
	}

	s := u.String()
	// Drop a dangling "?" left when every param was volatile.
	s = strings.TrimSuffix(s, "?")
	return s
}

// normalizeTitleKey reduces a title to letters and digits only (lowercased)
// so exact cross-posts — the same headline copied to multiple communities —
// collapse to one key regardless of bracket tags, spacing, or punctuation.
// It is intentionally conservative: it matches identical headlines, not
// merely same-topic ones, to avoid dropping distinct posts.
func normalizeTitleKey(title string) string {
	var b strings.Builder
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
