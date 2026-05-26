package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
)

// slackPayload is a Slack-compatible incoming webhook payload.
// Also works with Discord and other Slack-format webhook services.
type slackPayload struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}

type slackBlock struct {
	Type string        `json:"type"`
	Text *slackText    `json:"text,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// WebhookNotifier sends notifications via an HTTP webhook (Slack / Discord / custom).
type WebhookNotifier struct {
	url        string
	httpClient *http.Client
}

// NewWebhookNotifier creates a WebhookNotifier.
// Returns nil when url is empty (disabled).
func NewWebhookNotifier(url string) *WebhookNotifier {
	if url == "" {
		return nil
	}
	return &WebhookNotifier{
		url: url,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Alert sends a notification webhook. Best-effort — no retry.
func (n *WebhookNotifier) Alert(ctx context.Context, event notify.NotificationEvent) error {
	payload := buildSlackPayload(event)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// buildSlackPayload creates a human-readable Slack message from a NotificationEvent.
func buildSlackPayload(event notify.NotificationEvent) slackPayload {
	var emoji, title string
	switch event.Type {
	case "job_failure":
		emoji = "⚠️"
		title = "KidSaber API Job *failure*"
	case "pool_low":
		emoji = "🔶"
		title = "KidSaber API *question pool is low*"
	case "job_success":
		emoji = "✅"
		title = "KidSaber API Job *completed successfully*"
	default:
		emoji = "ℹ️"
		title = "KidSaber API notification"
	}

	text := fmt.Sprintf("%s %s", emoji, title)

	if event.FailedCount > 0 {
		text += fmt.Sprintf(" — *%d combination(s) failed*", event.FailedCount)
	}

	if event.JobRunID != "" {
		text += fmt.Sprintf(" (job `%s`)", event.JobRunID)
	}

	var detailLines []string
	for _, d := range event.Details {
		detailLines = append(detailLines,
			fmt.Sprintf("• %s / grade %d / %s: %s", d.Subject, d.Grade, d.Type, d.Error))
	}

	blocks := []slackBlock{
		{
			Type: "section",
			Text: &slackText{
				Type: "mrkdwn",
				Text: text + func() string {
					if len(detailLines) > 0 {
						return "\n" + strings.Join(detailLines, "\n")
					}
					return ""
				}(),
			},
		},
	}

	return slackPayload{Text: text, Blocks: blocks}
}
