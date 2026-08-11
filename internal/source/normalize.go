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
