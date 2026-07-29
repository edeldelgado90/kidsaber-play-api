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

// discordPayload is a Discord incoming webhook payload.
type discordPayload struct {
	Content string `json:"content"`
}

// WebhookNotifier sends notifications via a Discord incoming webhook.
type WebhookNotifier struct {
	url        string
	httpClient *http.Client
}

// NewWebhookNotifier creates a WebhookNotifier for Discord.
// Returns nil (as NotificationService) when url is empty so MultiNotifier can filter it out safely.
func NewWebhookNotifier(url string) notify.NotificationService {
	if url == "" {
		return nil
	}
	return &WebhookNotifier{
		url:        url,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Alert sends a notification webhook. Best-effort — no retry.
func (n *WebhookNotifier) Alert(ctx context.Context, event notify.NotificationEvent) error {
	payload := buildDiscordPayload(event)

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

// buildDiscordPayload creates a human-readable Discord message from a NotificationEvent.
func buildDiscordPayload(event notify.NotificationEvent) discordPayload {
	if event.Type == notify.EventQuestionReport && event.Report != nil {
		return buildReportPayload(event.Report)
	}

	var emoji, title string
	switch event.Type {
	case notify.EventJobFailure:
		emoji = "⚠️"
		title = "KidSaber API Job **failure**"
	case notify.EventPoolLow:
		emoji = "🔶"
		title = "KidSaber API **question pool is low**"
	case notify.EventJobSuccess:
		emoji = "✅"
		title = "KidSaber API Job **completed successfully**"
	default:
		emoji = "ℹ️"
		title = "KidSaber API notification"
	}

	content := fmt.Sprintf("%s %s", emoji, title)

	if event.FailedCount > 0 {
		content += fmt.Sprintf(" — **%d combination(s) failed**", event.FailedCount)
	}

	if event.JobRunID != "" {
		content += fmt.Sprintf(" (job `%s`)", event.JobRunID)
	}

	var detailLines []string
	for _, d := range event.Details {
		detailLines = append(detailLines,
			fmt.Sprintf("• %s / grade %d / %s: %s", d.Subject, d.Grade, d.Type, d.Error))
	}

	if len(detailLines) > 0 {
		content += "\n" + strings.Join(detailLines, "\n")
	}

	return discordPayload{Content: content}
}

// maxStatementLen caps the statement copied into Discord. Statements are short
// by construction, so this only guards against a pathological stored value.
const maxStatementLen = 300

// buildReportPayload renders a player report as a Discord message.
//
// The statement is machine-generated content from the question bank, but it
// still reaches a channel that pings humans, so it is neutralised rather than
// trusted: mentions are defused and the text is fenced so markdown inside it
// cannot restyle the message or forge the rest of the alert.
func buildReportPayload(r *notify.ReportDetail) discordPayload {
	content := fmt.Sprintf(
		"🚩 **Pregunta reportada** — %s / %d.º / %s\n```\n%s\n```\nID: `%s`",
		sanitizeDiscord(r.Subject),
		r.Grade,
		sanitizeDiscord(r.Type),
		sanitizeDiscord(truncate(r.Statement, maxStatementLen)),
		sanitizeDiscord(r.QuestionID),
	)

	if r.ReportCount > 1 {
		content += fmt.Sprintf(" · %d reportes", r.ReportCount)
	}

	return discordPayload{Content: content}
}

// sanitizeDiscord defuses text before it is embedded in a webhook message.
//
// A zero-width space after "@" stops @everyone, @here and role pings from
// resolving while leaving the text readable, and stripping backticks prevents
// the value from closing the code fence it is rendered inside.
func sanitizeDiscord(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "@", "@​")
	return s
}

// truncate shortens s to at most n runes, appending an ellipsis when it cuts.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
