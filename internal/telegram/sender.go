package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bc1qwerty/best-archive-bot/internal/config"
)

const apiBase = "https://api.telegram.org/bot"

// linkPreviewOptions for Telegram API.
type linkPreviewOptions struct {
	PreferLargeMedia bool `json:"prefer_large_media"`
	ShowAboveText    bool `json:"show_above_text"`
}

// sendMessageRequest is the Telegram sendMessage payload.
type sendMessageRequest struct {
	ChatID              string             `json:"chat_id"`
	Text                string             `json:"text"`
	DisableNotification bool               `json:"disable_notification"`
	LinkPreviewOptions  linkPreviewOptions `json:"link_preview_options"`
}

// apiResponse is a minimal Telegram API response.
type apiResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after,omitempty"`
	} `json:"parameters,omitempty"`
}

// SendMessage sends a text message to the configured chat.
// It retries up to maxRetry times on failure, with special handling for rate limits.
func SendMessage(client *http.Client, text string, maxRetry int) error {
	payload := sendMessageRequest{
		ChatID:              config.ChatID,
		Text:                text,
		DisableNotification: true,
		LinkPreviewOptions: linkPreviewOptions{
			PreferLargeMedia: true,
			ShowAboveText:    false,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	apiURL := apiBase + config.BotToken + "/sendMessage"

	for attempt := 1; attempt <= maxRetry; attempt++ {
		req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(body)))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("전송 실패 (%d/%d): %v", attempt, maxRetry, err)
			if attempt < maxRetry {
				time.Sleep(3 * time.Second)
			}
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var apiResp apiResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			log.Printf("응답 파싱 실패 (%d/%d): %v", attempt, maxRetry, err)
			if attempt < maxRetry {
				time.Sleep(3 * time.Second)
			}
			continue
		}

		if apiResp.OK {
			return nil
		}

		// Handle rate limit
		if apiResp.Parameters != nil && apiResp.Parameters.RetryAfter > 0 {
			wait := time.Duration(apiResp.Parameters.RetryAfter+1) * time.Second
			log.Printf("Rate limit, %d초 대기", apiResp.Parameters.RetryAfter)
			time.Sleep(wait)
			continue
		}

		log.Printf("Telegram API 에러 (%d/%d): %s", attempt, maxRetry, apiResp.Description)
		if attempt < maxRetry {
			time.Sleep(3 * time.Second)
		}
	}

	return fmt.Errorf("전송 최종 실패 after %d attempts", maxRetry)
}

// SendPlainMessage sends a simple notification (e.g., for admin alerts).
func SendPlainMessage(client *http.Client, chatID, text string) error {
	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("text", text)

	apiURL := apiBase + config.BotToken + "/sendMessage"
	resp, err := client.PostForm(apiURL, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
