package notify

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
	"sync"
	"time"

	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
)

// SMTPNotifier sends email notifications via SMTP using Go stdlib net/smtp.
// When dailyLimit > 0 the notifier tracks how many emails have been sent
// on the current UTC day and silently drops further sends once the limit
// is reached, resetting the counter automatically at midnight UTC.
type SMTPNotifier struct {
	host       string
	port       int
	user       string
	password   string
	from       string
	to         []string
	dailyLimit int

	mu        sync.Mutex
	sentToday int
	lastDay   time.Time // UTC day of the last successful send (or initialisation)
}

// NewSMTPNotifier creates an SMTPNotifier.
// Returns nil (as NotificationService) when host is empty so MultiNotifier can filter it out safely.
// dailyLimit caps outgoing emails per UTC day; 0 means no cap.
func NewSMTPNotifier(host string, port int, user, password, from, to string, dailyLimit int) unotify.NotificationService {
	if host == "" {
		return nil
	}
	return &SMTPNotifier{
		host:       host,
		port:       port,
		user:       user,
		password:   password,
		from:       from,
		to:         parseRecipients(to),
		dailyLimit: dailyLimit,
		lastDay:    today(),
	}
}

// today returns the current UTC date truncated to midnight.
func today() time.Time {
	return time.Now().UTC().Truncate(24 * time.Hour)
}

// withinDailyLimit checks whether another email can be sent and increments the
// counter if so. Returns false (and the remaining budget) when the limit is
// exceeded. The caller must not hold n.mu when calling this.
func (n *SMTPNotifier) withinDailyLimit() bool {
	if n.dailyLimit == 0 {
		return true
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if t := today(); t.After(n.lastDay) {
		n.sentToday = 0
		n.lastDay = t
	}

	if n.sentToday >= n.dailyLimit {
		return false
	}
	n.sentToday++
	return true
}

// Alert sends a notification email. Best-effort — no retry.
// ctx is checked for cancellation before sending.
// Returns an error (without sending) when the daily limit has been reached.
func (n *SMTPNotifier) Alert(ctx context.Context, event unotify.NotificationEvent) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !n.withinDailyLimit() {
		return fmt.Errorf("SMTP daily limit of %d emails reached; skipping notification", n.dailyLimit)
	}

	subject, plainBody, htmlBody := buildEmailContent(event)
	msg := buildMIMEMessage(n.from, n.to, subject, plainBody, htmlBody)

	addr := fmt.Sprintf("%s:%d", n.host, n.port)
	auth := smtp.PlainAuth("", n.user, n.password, n.host)

	if err := smtp.SendMail(addr, auth, n.from, n.to, msg); err != nil {
		return fmt.Errorf("sending SMTP notification: %w", err)
	}

	return nil
}

// buildEmailContent returns the email subject and bodies for a NotificationEvent.
func buildEmailContent(event unotify.NotificationEvent) (subject, plain, htmlBody string) {
	switch event.Type {
	case "job_failure":
		subject = fmt.Sprintf("[KidSaber API] Job failure — %d combination(s) failed", event.FailedCount)
	case "pool_low":
		subject = "[KidSaber API] Question pool low"
	case "job_success":
		subject = "[KidSaber API] Job completed successfully"
	default:
		subject = "[KidSaber API] Notification"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Event: %s\n", event.Type))
	sb.WriteString(fmt.Sprintf("Time:  %s\n", event.Timestamp.Format(time.RFC3339)))
	if event.JobRunID != "" {
		sb.WriteString(fmt.Sprintf("Job:   %s\n", event.JobRunID))
	}
	if event.FailedCount > 0 {
		sb.WriteString(fmt.Sprintf("Failed combinations: %d\n\n", event.FailedCount))
		for _, d := range event.Details {
			sb.WriteString(fmt.Sprintf("  • %s / grade %d / %s\n    Error: %s\n",
				d.Subject, d.Grade, d.Type, d.Error))
		}
	}
	plain = sb.String()

	// Simple HTML version
	const htmlTmpl = `<html><body>
<h2>KidSaber API — {{.Type}}</h2>
<p><strong>Time:</strong> {{.Time}}</p>
{{if .JobRunID}}<p><strong>Job ID:</strong> {{.JobRunID}}</p>{{end}}
{{if .Details}}<h3>Failed combinations ({{.FailedCount}}):</h3><ul>
{{range .Details}}<li><strong>{{.Subject}} / grade {{.Grade}} / {{.Type}}</strong><br>{{.Error}}</li>{{end}}
</ul>{{end}}
</body></html>`

	tmpl, err := template.New("email").Parse(htmlTmpl)
	if err == nil {
		data := struct {
			Type        string
			Time        string
			JobRunID    string
			FailedCount int
			Details     []unotify.FailureDetail
		}{
			Type:        event.Type,
			Time:        event.Timestamp.Format(time.RFC3339),
			JobRunID:    event.JobRunID,
			FailedCount: event.FailedCount,
			Details:     event.Details,
		}
		var buf bytes.Buffer
		if tmpl.Execute(&buf, data) == nil {
			htmlBody = buf.String()
		}
	}

	return
}

// buildMIMEMessage constructs a multipart/alternative MIME email.
func buildMIMEMessage(from string, to []string, subject, plain, htmlBody string) []byte {
	var buf bytes.Buffer
	boundary := "kidsaber-boundary-001"

	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
	buf.WriteString("\r\n")

	// Plain text part
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(plain)
	buf.WriteString("\r\n")

	// HTML part
	if htmlBody != "" {
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		buf.WriteString(htmlBody)
		buf.WriteString("\r\n")
	}

	buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return buf.Bytes()
}

// parseRecipients splits a comma-separated recipient string into a slice.
func parseRecipients(s string) []string {
	var result []string
	for _, r := range strings.Split(s, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			result = append(result, r)
		}
	}
	return result
}
