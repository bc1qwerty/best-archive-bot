package bot

import (
	"fmt"
	"strings"

	"github.com/bc1qwerty/best-archive-bot/internal/scraper"
)

// FormatSinglePost formats a post for Telegram message.
// Example: [인벤] 게시물제목 ❤84 👁12,000
// https://www.inven.co.kr/...
func FormatSinglePost(post scraper.Post) string {
	var stats []string
	if post.Votes > 0 {
		stats = append(stats, fmt.Sprintf("❤%d", post.Votes))
	}
	if post.Views > 0 {
		stats = append(stats, fmt.Sprintf("👁%s", formatNumber(post.Views)))
	}
	if post.Comments > 0 {
		stats = append(stats, fmt.Sprintf("💬%d", post.Comments))
	}

	suffix := ""
	if len(stats) > 0 {
		suffix = " " + strings.Join(stats, " ")
	}

	return fmt.Sprintf("[%s] %s%s\n%s", post.CommunityName, post.Title, suffix, post.URL)
}

// formatNumber formats an integer with comma separators (e.g., 12000 → "12,000").
func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
