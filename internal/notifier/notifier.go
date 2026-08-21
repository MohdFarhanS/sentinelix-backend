package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// MultiNotifier mengimplementasikan usecase.Notifier. Dispatch berdasarkan
// rule.Channel — satu rule cuma punya SATU channel (sesuai skema
// alert_rules.channel), jadi tidak ada fan-out ke email+slack sekaligus
// dari satu rule yang sama.
type MultiNotifier struct {
	resendAPIKey	string
	emailFrom		string
	httpClient		*http.Client
}

func NewMultiNotifier(resendAPIKey, emailFrom string) *MultiNotifier {
	return &MultiNotifier{
		resendAPIKey: 	resendAPIKey,
		emailFrom: 		emailFrom,
		httpClient: 	&http.Client{Timeout: 10 * time.Second},
	}
}

func (n *MultiNotifier) Notify(ctx context.Context, rule *domain.AlertRule, issue *domain.Issue) error {
	switch rule.Channel {
	case domain.ChannelEmail:
		return n.sendEmail(ctx, rule.ChannelTarget, issue)
	case domain.ChannelSlack:
		return n.sendSlack(ctx, rule.ChannelTarget, issue)
	default:
		return fmt.Errorf("notifier: unknown channel %q", rule.Channel)
	}
}

type resendEmailPayload struct {
	From		string		`json:"from"`
	To			[]string	`json:"to"`
	Subject		string		`json:"subject"`
	Html		string		`json:"html"`
}

// sendEmail panggil Resend REST API langsung via net/http (bukan pakai
// resend-go SDK) — YAGNI, cuma 1 endpoint yang dipakai, tidak perlu nambah
// dependency baru untuk itu.
func (n *MultiNotifier) sendEmail(ctx context.Context, to string, issue *domain.Issue) error {
	body := resendEmailPayload{
		From: n.emailFrom,
		To: []string{to},
		Subject: fmt.Sprintf("[SentinelIX] Alert: %s", issue.Title),
		Html: fmt.Sprintf(
			"<p>Issue <b>%s<b> triggered an alert.</p><p>Count: %d</p><p>Last seen: %s</p>",
			issue.Title, issue.Count, issue.LastSeen.UTC().Format(time.RFC3339),
		),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+n.resendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("resend API returned status %d", resp.StatusCode)
	}
	return nil
}

type slackPayload struct {
	Text string	`json:"text"`
}

func (n *MultiNotifier) sendSlack(ctx context.Context, webhookURL string, issue *domain.Issue) error {
	body := slackPayload{
		Text: fmt.Sprintf(
			":rotating_light: *SentinelIX Alert*\nIssue: %s\nCount: %d\nLast seen: %s",
			issue.Title, issue.Count, issue.LastSeen.UTC().Format(time.RFC3339),
		),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// NotifyMonitorDown dipanggil MonitorCheckerUsecase (lihat
// usecase/check_monitor.go) begitu consecutive failure count menyentuh
// FailureThreshold. consecutiveFailures dikirim terpisah dari monitor
// (bukan field di domain.Monitor) karena keputusan arsitektur kita:
// on-the-fly query dari monitor_checks, bukan counter kolom — jadi
// usecase yang tahu angkanya, bukan struct Monitor itu sendiri.
func (n *MultiNotifier) NotifyMonitorDown(ctx context.Context, monitor *domain.Monitor, consecutiveFailures int) error {
	switch monitor.Channel {
	case domain.ChannelEmail:
		return n.sendMonitorDownEmail(ctx, monitor.ChannelTarget, monitor, consecutiveFailures)
	case domain.ChannelSlack:
		return n.sendMonitorDownSlack(ctx, monitor.ChannelTarget, monitor, consecutiveFailures)
	default:
		return fmt.Errorf("notifier: unknown channel %q", monitor.Channel)
	}
}

func (n *MultiNotifier) sendMonitorDownEmail(ctx context.Context, to string, monitor *domain.Monitor, consecutiveFailures int) error {
	body := resendEmailPayload{
		From:    n.emailFrom,
		To:      []string{to},
		Subject: fmt.Sprintf("[SentinelIX] Monitor DOWN: %s", monitor.URL),
		Html: fmt.Sprintf(
			"<p>Monitor <b>%s</b> is currently <b>DOWN</b>.</p><p>Failed %d consecutive checks (threshold: %d).</p>",
			monitor.URL, consecutiveFailures, monitor.FailureThreshold,
		),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+n.resendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("resend API returned status %d", resp.StatusCode)
	}
	return nil
}

func (n *MultiNotifier) sendMonitorDownSlack(ctx context.Context, webhookURL string, monitor *domain.Monitor, consecutiveFailures int) error {
	body := slackPayload{
		Text: fmt.Sprintf(
			":red_circle: *SentinelIX Monitor DOWN*\nURL: %s\nConsecutive failures: %d (threshold: %d)",
			monitor.URL, consecutiveFailures, monitor.FailureThreshold,
		),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}