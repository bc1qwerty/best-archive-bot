package source

import (
	"context"
	"errors"
	"fmt"

	"github.com/bc1qwerty/txid-bot-framework/pkg/core"
)

// InterleavingSource fetches items from multiple sub-sources and merges
// them in round-robin order rather than concatenation. This prevents a
// single chatty community from monopolizing the dispatch slots and
// preserves the per-community fairness the pre-framework bot enforced.
//
// Sub-source failures are non-fatal: errors are joined and returned
// alongside whatever items came back, mirroring core.MultiSource so the
// Runner can dispatch partial results.
type InterleavingSource struct {
	Sources []core.Source
	// MaxPerSource caps how many items each sub-source contributes per
	// fetch. 0 means no per-source cap (use the framework's global
	// MaxItemsPerPoll instead).
	MaxPerSource int
}

func NewInterleavingSource(sources ...core.Source) *InterleavingSource {
	return &InterleavingSource{Sources: sources}
}

// Name returns a stable identifier so bot_seen keys remain consistent
// regardless of sub-source ordering changes.
func (s *InterleavingSource) Name() string {
	return "best-archive-interleaved"
}

// Fetch pulls from each sub-source then interleaves: pick #0 from each
// in order, then #1 from each, etc. Sub-sources that ran out keep being
// skipped until the longest queue is drained.
func (s *InterleavingSource) Fetch(ctx context.Context) ([]core.Item, error) {
	buckets := make([][]core.Item, 0, len(s.Sources))
	var errs []error
	maxLen := 0

	for _, src := range s.Sources {
		items, err := src.Fetch(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("source %s: %w", src.Name(), err))
		}
		if s.MaxPerSource > 0 && len(items) > s.MaxPerSource {
			items = items[:s.MaxPerSource]
		}
		buckets = append(buckets, items)
		if len(items) > maxLen {
			maxLen = len(items)
		}
	}

	out := make([]core.Item, 0)
	for i := 0; i < maxLen; i++ {
		for _, b := range buckets {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}

	if len(errs) > 0 {
		return out, errors.Join(errs...)
	}
	return out, nil
}
