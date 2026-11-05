package scraper

import "testing"

// 키가 어긋나면 그 커뮤니티는 조용히 전역 기준으로 돌아간다(2026-11-06 «82cook» 이
// 실제 키 «cook82» 와 달라 하루 동안 적용되지 않았다).
func TestThresholdKeys(t *testing.T) {
	known := map[string]bool{}
	for _, s := range AllScrapers() {
		known[s.(interface{ base() *baseScraper }).base().community] = true
	}
	for k := range communityThresholds {
		if !known[k] {
			t.Errorf("communityThresholds[%q] 에 해당하는 스크래퍼가 없다", k)
		}
	}
}
