package scraper

import (
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/bc1qwerty/best-archive-bot/internal/config"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36 Edg/133.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.3 Safari/605.1.15",
}

// semaphore limits concurrent HTTP requests.
var semaphore chan struct{}
var semOnce sync.Once

func getSemaphore() chan struct{} {
	semOnce.Do(func() {
		semaphore = make(chan struct{}, config.MaxConcurrentRequests)
	})
	return semaphore
}

// NewHTTPClient creates an http.Client with TLS skip-verify and 15s timeout.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

// randomDelay sleeps for a random duration between RequestDelayMin and RequestDelayMax.
func randomDelay() {
	min := config.RequestDelayMin
	max := config.RequestDelayMax
	d := min + rand.Float64()*(max-min)
	time.Sleep(time.Duration(d * float64(time.Second)))
}

const (
	// fetchMaxAttempts is the total number of GET tries per URL (1 initial
	// attempt + 2 retries) for transient transport errors, HTTP 429, and 5xx.
	fetchMaxAttempts = 3
	// retryAfterCap bounds how long a server's Retry-After header is honored.
	// The cron tick comes around again soon, so sleeping minutes inside one
	// run just burns the GHA budget.
	retryAfterCap = 10 * time.Second
)

// retryableStatus reports whether an HTTP status warrants a retry: 429 (rate
// limited) and 5xx (server-side, usually transient). Other 4xx are permanent.
func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}

// retryAfterDelay extracts a bounded delay from a Retry-After header. Only the
// integer delta-seconds form is honored; an HTTP-date form yields 0 so the
// caller falls back to its own backoff.
func retryAfterDelay(resp *http.Response) time.Duration {
	v := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		if d := time.Duration(secs) * time.Second; d < retryAfterCap {
			return d
		}
		return retryAfterCap
	}
	return 0
}

// backoffDelay returns the wait before the next retry (attempt is 1-based).
func backoffDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 2 * time.Second
	}
	return 5 * time.Second
}

// fetchHTML fetches a URL and returns the raw HTML string. encoding can be
// "utf-8" or "euc-kr". Transient failures (transport errors, HTTP 429, 5xx)
// are retried with backoff up to fetchMaxAttempts; a 429/503 Retry-After
// header is honored when present (capped at retryAfterCap).
func fetchHTML(client *http.Client, url, encoding, referer string) (string, error) {
	sem := getSemaphore()
	sem <- struct{}{}
	defer func() { <-sem }()

	randomDelay()

	var lastErr error
	for attempt := 1; attempt <= fetchMaxAttempts; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7")
		if referer != "" {
			req.Header.Set("Referer", referer)
		}

		resp, err := client.Do(req)
		if err != nil {
			// Transport-level failure (timeout, connection reset) — retry.
			lastErr = fmt.Errorf("GET %s: %w", url, err)
			if attempt < fetchMaxAttempts {
				time.Sleep(backoffDelay(attempt))
				continue
			}
			return "", lastErr
		}

		if resp.StatusCode != http.StatusOK {
			status := resp.StatusCode
			wait := retryAfterDelay(resp)
			resp.Body.Close()
			lastErr = fmt.Errorf("GET %s: status %d", url, status)
			if retryableStatus(status) && attempt < fetchMaxAttempts {
				if wait == 0 {
					wait = backoffDelay(attempt)
				}
				time.Sleep(wait)
				continue
			}
			return "", lastErr
		}

		var body []byte
		if strings.EqualFold(encoding, "euc-kr") {
			reader := transform.NewReader(resp.Body, korean.EUCKR.NewDecoder())
			body, err = io.ReadAll(reader)
		} else {
			body, err = io.ReadAll(resp.Body)
		}
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("read body %s: %w", url, err)
		}
		return string(body), nil
	}
	return "", lastErr
}

// fetchDocument fetches a URL and returns a goquery Document.
func fetchDocument(client *http.Client, url, encoding, referer string) (*goquery.Document, error) {
	html, err := fetchHTML(client, url, encoding, referer)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse HTML %s: %w", url, err)
	}
	return doc, nil
}
