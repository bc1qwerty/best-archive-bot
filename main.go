package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/bc1qwerty/best-archive-bot/internal/bot"
	"github.com/bc1qwerty/best-archive-bot/internal/config"
	"github.com/bc1qwerty/best-archive-bot/internal/db"
	"github.com/bc1qwerty/best-archive-bot/internal/scraper"
	"github.com/bc1qwerty/best-archive-bot/internal/notifyhub"
	"github.com/bc1qwerty/best-archive-bot/internal/telegram"
)

const (
	sendDelay     = 3 * time.Second
	maxSendPerRun = 10
	backlogCap    = 20
	maxSendRetry  = 2
	scrapeTimeout = 60 * time.Second
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.Println("=== run_once 시작 ===")
	notifyhub.LogPush("best-archive-bot", "info", "run started", "")

	// Determine base directory (where main.go lives)
	exe, err := os.Executable()
	if err != nil {
		// Fallback to working directory
		exe, _ = os.Getwd()
	}
	baseDir := filepath.Dir(exe)
	// If running with `go run`, use working directory instead
	if _, err := os.Stat(filepath.Join(baseDir, "config", "communities.yaml")); err != nil {
		baseDir, _ = os.Getwd()
	}

	config.Load(baseDir)

	if config.BotToken == "" || config.BotToken == "your_bot_token_here" {
		log.Fatal("BOT_TOKEN이 설정되지 않음")
	}
	if config.ChatID == "" || config.ChatID == "your_chat_id_here" {
		log.Fatal("CHAT_ID가 설정되지 않음")
	}

	// DB init + cleanup
	postDB := db.New()
	if err := postDB.Init(); err != nil {
		log.Fatalf("DB 초기화 실패: %v", err)
	}
	defer postDB.Close()

	if err := postDB.CleanupOldRecords(); err != nil {
		log.Printf("DB 정리 실패: %v", err)
	}

	// Scrape all communities
	allPosts := scrapeAll()
	log.Printf("총 %d건 수집 완료", len(allPosts))

	// Filter unsent posts
	unsent, err := postDB.FilterUnsent(allPosts)
	if err != nil {
		log.Fatalf("미전송 필터 실패: %v", err)
	}
	log.Printf("미전송 게시글: %d건", len(unsent))

	// Interleave by community
	unsent = interleavePosts(unsent)

	// Backlog cap: mark excess as sent to prevent re-appearance
	if len(unsent) > backlogCap {
		skipped := unsent[backlogCap:]
		unsent = unsent[:backlogCap]
		if err := postDB.MarkSent(skipped); err != nil {
			log.Printf("백로그 스킵 기록 실패: %v", err)
		}
		log.Printf("백로그 초과: %d건 스킵 처리", len(skipped))
	}

	// Send posts
	sent := sendPosts(postDB, unsent)
	log.Printf("=== run_once 완료: %d건 전송 ===", sent)
	notifyhub.LogPush("best-archive-bot", "info", "run finished", fmt.Sprintf("sent=%d", sent))
}

// scraperList returns all enabled scrapers.
func scraperList() []scraper.Scraper {
	return []scraper.Scraper{
		scraper.NewDcinsideScraper(),
		scraper.NewTheqooScraper(),
		scraper.NewNatepannScraper(),
		scraper.NewClienScraper(),
		scraper.NewBobaedreamScraper(),
		scraper.NewMlbparkScraper(),
		scraper.NewPpomppuScraper(),
		scraper.NewRuliwebScraper(),
		scraper.NewInvenScraper(),
		scraper.NewCook82Scraper(),
		scraper.NewHumorunivScraper(),
		scraper.NewEtolandScraper(),
		scraper.NewDvdprimeScraper(),
	}
}

// scrapeAll runs all scrapers concurrently and collects results.
func scrapeAll() []scraper.Post {
	scrapers := scraperList()
	client := scraper.NewHTTPClient()

	type result struct {
		name  string
		posts []scraper.Post
	}

	results := make([]result, len(scrapers))
	var wg sync.WaitGroup

	for i, s := range scrapers {
		wg.Add(1)
		go func(idx int, s scraper.Scraper) {
			defer wg.Done()

			done := make(chan []scraper.Post, 1)
			go func() {
				posts, err := s.FetchBestPosts(client)
				if err != nil {
					log.Printf("[%s] 스크래핑 실패: %v", s.Name(), err)
					done <- nil
					return
				}
				done <- posts
			}()

			select {
			case posts := <-done:
				if posts != nil {
					log.Printf("[%s] %d건 수집", s.Name(), len(posts))
				}
				results[idx] = result{name: s.Name(), posts: posts}
			case <-time.After(scrapeTimeout):
				log.Printf("[%s] 타임아웃 (%s)", s.Name(), scrapeTimeout)
				results[idx] = result{name: s.Name()}
			}
		}(i, s)
	}

	wg.Wait()

	var allPosts []scraper.Post
	for _, r := range results {
		allPosts = append(allPosts, r.posts...)
	}
	return allPosts
}

// interleavePosts groups posts by community, shuffles within each group,
// then round-robins so the same community doesn't appear consecutively.
func interleavePosts(posts []scraper.Post) []scraper.Post {
	if len(posts) <= 1 {
		return posts
	}

	// Group by community
	byCommunity := make(map[string][]scraper.Post)
	for _, p := range posts {
		byCommunity[p.Community] = append(byCommunity[p.Community], p)
	}

	// Shuffle within each community
	for _, group := range byCommunity {
		rand.Shuffle(len(group), func(i, j int) {
			group[i], group[j] = group[j], group[i]
		})
	}

	// Sort buckets by size (largest first)
	buckets := make([][]scraper.Post, 0, len(byCommunity))
	for _, group := range byCommunity {
		buckets = append(buckets, group)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return len(buckets[i]) > len(buckets[j])
	})

	// Round-robin: take 1 from each bucket in turn
	var result []scraper.Post
	idx := 0
	for {
		// Remove empty buckets
		var active [][]scraper.Post
		for _, b := range buckets {
			if len(b) > 0 {
				active = append(active, b)
			}
		}
		buckets = active
		if len(buckets) == 0 {
			break
		}
		bi := idx % len(buckets)
		result = append(result, buckets[bi][0])
		buckets[bi] = buckets[bi][1:]
		idx++
	}

	log.Printf("큐 인터리빙 완료: %d건 (%d개 커뮤니티)", len(result), len(byCommunity))
	return result
}

// sendPosts sends unsent posts sequentially via Telegram.
func sendPosts(postDB *db.PostDB, posts []scraper.Post) int {
	if len(posts) == 0 {
		log.Println("전송할 게시글 없음")
		return 0
	}

	// Apply max-send limit
	if len(posts) > maxSendPerRun {
		log.Printf("전송 대상 %d건 → %d건으로 제한", len(posts), maxSendPerRun)
		posts = posts[:maxSendPerRun]
	}

	client := &http.Client{Timeout: 30 * time.Second}
	sentCount := 0

	for _, post := range posts {
		msg := bot.FormatSinglePost(post)
		err := telegram.SendMessage(client, msg, maxSendRetry)

		if err != nil {
			log.Printf("[%s] 전송 최종 실패: %s", post.CommunityName, post.Title)
			continue
		}

		// Push to notification hub
		if err := notifyhub.Push(notifyhub.Payload{
			ChannelID: "best-archive",
			Title:     post.Title,
			Body:      post.CommunityName,
			URL:       post.URL,
			Category:  post.CommunityName,
		}); err != nil {
			log.Printf("[%s] hub push 실패: %v", post.CommunityName, err)
		}

		// Mark as sent with retry
		for dbAttempt := 1; dbAttempt <= 3; dbAttempt++ {
			if err := postDB.MarkSent([]scraper.Post{post}); err != nil {
				log.Printf("[%s] mark_sent 실패 (%d/3): %s", post.CommunityName, dbAttempt, post.URL)
				if dbAttempt < 3 {
					time.Sleep(1 * time.Second)
				}
			} else {
				break
			}
		}

		sentCount++
		time.Sleep(sendDelay)
	}

	return sentCount
}
