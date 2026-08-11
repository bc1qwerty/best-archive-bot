package source

import (
	"sort"

	"github.com/bc1qwerty/best-archive-bot/internal/scraper"
)

// popularityScore normalizes each metric against the bucket's maximum and
// sums the normalized values. Normalizing per-bucket keeps the score
// meaningful within a single community regardless of which metric that
// community reports (votes vs views vs comments) and avoids one large-scale
// metric (views in the tens of thousands) dominating another (votes in the
// hundreds). Cross-community comparison is intentionally NOT attempted: the
// interleaving source already merges the sorted buckets round-robin.
func popularityScore(p scraper.Post, maxVotes, maxViews, maxComments int) float64 {
	var s float64
	if maxVotes > 0 {
		s += float64(p.Votes) / float64(maxVotes)
	}
	if maxViews > 0 {
		s += float64(p.Views) / float64(maxViews)
	}
	if maxComments > 0 {
		s += float64(p.Comments) / float64(maxComments)
	}
	return s
}

// sortByPopularity stably orders a single community's posts hottest-first so
// the interleaving source picks each community's best posts before its
// weaker ones. Posts with no metrics (0/0/0) keep their original scraped
// order via the stable sort.
func sortByPopularity(posts []scraper.Post) {
	maxVotes, maxViews, maxComments := 0, 0, 0
	for _, p := range posts {
		if p.Votes > maxVotes {
			maxVotes = p.Votes
		}
		if p.Views > maxViews {
			maxViews = p.Views
		}
		if p.Comments > maxComments {
			maxComments = p.Comments
		}
	}
	sort.SliceStable(posts, func(i, j int) bool {
		return popularityScore(posts[i], maxVotes, maxViews, maxComments) >
			popularityScore(posts[j], maxVotes, maxViews, maxComments)
	})
}
