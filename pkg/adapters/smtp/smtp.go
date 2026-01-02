package smtp

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/mail"
	gosmtp "net/smtp"
	"strings"
	"time"

	"github.com/jaytaylor/html2text"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter delivers email using an SMTP server with optional TLS/STARTTLS.
type Adapter struct {
	name string
	base adapters.BaseAdapter
	caps adapters.Capability
	cfg  Config
}

type Option func(*Adapter)

// Config captures connection/auth options for SMTP.
type Config struct {
	Host          string
	Port          int
	Username      string
	Password      string
	From          string
	UseTLS        bool
	UseStartTLS   bool
	SkipTLSVerify bool
	Timeout       time.Duration
	LocalName     string
	AuthDisabled  bool
	Headers       map[string]string
	PlainOnly     bool // Force text/plain even when HTML is available.
}

// WithName overrides the provider name (defaults to smtp).
func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

// WithConfig sets the adapter configuration.
func WithConfig(cfg Config) Option {
	return func(a *Adapter) {
		a.cfg = cfg
	}
}

// WithCredentials configures username/password auth.
func WithCredentials(username, password string) Option {
	return func(a *Adapter) {
		a.cfg.Username = username
		a.cfg.Password = password
	}
}

// WithHostPort sets host and port.
func WithHostPort(host string, port int) Option {
	return func(a *Adapter) {
		if host != "" {
			a.cfg.Host = host
		}
		if port > 0 {
			a.cfg.Port = port
		}
	}
}

// WithFrom sets the default From address.
func WithFrom(from string) Option {
	return func(a *Adapter) {
		if from != "" {
			a.cfg.From = from
		}
	}
}

// WithTLS toggles implicit TLS.
func WithTLS(enabled bool) Option {
	return func(a *Adapter) {
		a.cfg.UseTLS = enabled
	}
}

// WithStartTLS toggles STARTTLS upgrade (defaults to true when not using implicit TLS).
func WithStartTLS(enabled bool) Option {
	return func(a *Adapter) {
		a.cfg.UseStartTLS = enabled
	}
}

func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "smtp",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "smtp",
			Channels: []string{"email"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			Port:        587,
			UseStartTLS: true,
			Timeout:     10 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	if strings.TrimSpace(a.cfg.Host) == "" {
		return fmt.Errorf("smtp: host is required")
	}
	if a.cfg.Port == 0 {
		a.cfg.Port = 587
	}

	from := firstNonEmpty(msg.Metadata, "from", a.cfg.From)
	if from == "" {
		return fmt.Errorf("smtp: from address is required")
	}
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("smtp: invalid from address: %w", err)
	}
	toAddr, err := mail.ParseAddress(msg.To)
	if err != nil {
		return fmt.Errorf("smtp: invalid to address: %w", err)
	}

	textBody := firstNonEmpty(msg.Metadata, "text_body", "text", "body", msg.Body)
	htmlBody := firstNonEmpty(msg.Metadata, "html_body", "html")
	if htmlBody != "" && strings.TrimSpace(textBody) == "" {
		textBody = htmlToText(htmlBody)
	}
	contentType := strings.TrimSpace(stringValue(msg.Metadata, "content_type"))

	body, headers := buildMessage(fromAddr.String(), toAddr.String(), msg.Subject, msg.Headers, a.cfg.Headers, textBody, htmlBody, contentType, a.cfg.PlainOnly, msg.Attachments)

	addr := fmt.Sprintf("%s:%d", a.cfg.Host, a.cfg.Port)
	dialer := &net.Dialer{Timeout: a.cfg.Timeout}
	tlsCfg := &tls.Config{
		ServerName:         a.cfg.Host,
		InsecureSkipVerify: a.cfg.SkipTLSVerify,
	}

	client, conn, err := a.newClient(ctx, dialer, addr, tlsCfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Quit()
		_ = conn.Close()
	}()

	if a.cfg.UseStartTLS && !a.cfg.UseTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("smtp: starttls failed: %w", err)
			}
		}
	}

	if !a.cfg.AuthDisabled && a.cfg.Username != "" {
		auth := gosmtp.PlainAuth("", a.cfg.Username, a.cfg.Password, a.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp: auth failed: %w", err)
		}
	}

	if err := client.Mail(fromAddr.Address); err != nil {
		return fmt.Errorf("smtp: mail from failed: %w", err)
	}
	if err := client.Rcpt(toAddr.Address); err != nil {
		return fmt.Errorf("smtp: rcpt to failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: open data: %w", err)
	}
	if _, err := w.Write([]byte(headers + "\r\n\r\n" + body)); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp: write data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: close data: %w", err)
	}

	a.base.LogSuccess(a.name, msg)
	return nil
}

func (a *Adapter) newClient(ctx context.Context, dialer *net.Dialer, addr string, tlsCfg *tls.Config) (*gosmtp.Client, net.Conn, error) {
	if a.cfg.UseTLS {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("smtp: tls dial failed: %w", err)
		}
		client, err := gosmtp.NewClient(conn, a.cfg.Host)
		if err != nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("smtp: new client failed: %w", err)
		}
		return client, conn, nil
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("smtp: dial failed: %w", err)
	}
	client, err := gosmtp.NewClient(conn, a.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("smtp: new client failed: %w", err)
	}
	return client, conn, nil
}

func buildMessage(from, to, subject string, msgHeaders map[string]string, cfgHeaders map[string]string, textBody, htmlBody, contentType string, plainOnly bool, attachments []adapters.Attachment) (string, string) {
	headers := map[string]string{
		"From":         from,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
	}
	for k, v := range cfgHeaders {
		headers[k] = v
	}
	for k, v := range msgHeaders {
		if v == "" {
			continue
		}
		headers[k] = v
	}
	if htmlBody != "" && strings.TrimSpace(textBody) == "" {
		textBody = htmlToText(htmlBody)
	}

	attachments = adapters.EmailAttachments(attachments)
	if len(attachments) > 0 {
		return buildMessageWithAttachments(headers, textBody, htmlBody, contentType, plainOnly, attachments)
	}

	if plainOnly {
		ct := contentType
		if ct == "" {
			ct = "text/plain; charset=UTF-8"
		}
		headers["Content-Type"] = ct
		return textBody, formatHeaders(headers)
	}

	if htmlBody != "" {
		boundary := fmt.Sprintf("mixed-%d", time.Now().UnixNano())
		headers["Content-Type"] = fmt.Sprintf("multipart/alternative; boundary=%s", boundary)

		var sb strings.Builder
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		sb.WriteString(textBody + "\r\n")
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		sb.WriteString(htmlBody + "\r\n")
		sb.WriteString("--" + boundary + "--")
		return sb.String(), formatHeaders(headers)
	}

	ct := contentType
	if ct == "" {
		ct = "text/plain; charset=UTF-8"
	}
	headers["Content-Type"] = ct
	return textBody, formatHeaders(headers)
}

func buildMessageWithAttachments(headers map[string]string, textBody, htmlBody, contentType string, plainOnly bool, attachments []adapters.Attachment) (string, string) {
	mixedBoundary := fmt.Sprintf("mixed-%d", time.Now().UnixNano())
	headers["Content-Type"] = fmt.Sprintf("multipart/mixed; boundary=%s", mixedBoundary)

	var sb strings.Builder
	writeBodyPart(&sb, mixedBoundary, textBody, htmlBody, contentType, plainOnly)
	for _, att := range attachments {
		ct := strings.TrimSpace(att.ContentType)
		if ct == "" {
			ct = "application/octet-stream"
		}
		sb.WriteString("--" + mixedBoundary + "\r\n")
		fmt.Fprintf(&sb, "Content-Type: %s\r\n", ct)
		fmt.Fprintf(&sb, "Content-Disposition: attachment; filename=\"%s\"\r\n", att.Filename)
		sb.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		sb.WriteString(encodeBase64Lines(att.Content))
		sb.WriteString("\r\n")
	}
	sb.WriteString("--" + mixedBoundary + "--")

	return sb.String(), formatHeaders(headers)
}

func writeBodyPart(sb *strings.Builder, mixedBoundary, textBody, htmlBody, contentType string, plainOnly bool) {
	if plainOnly {
		ct := contentType
		if ct == "" {
			ct = "text/plain; charset=UTF-8"
		}
		sb.WriteString("--" + mixedBoundary + "\r\n")
		fmt.Fprintf(sb, "Content-Type: %s\r\n\r\n", ct)
		sb.WriteString(textBody + "\r\n")
		return
	}

	if htmlBody != "" {
		if strings.TrimSpace(textBody) == "" {
			textBody = htmlToText(htmlBody)
		}
		altBoundary := fmt.Sprintf("alt-%d", time.Now().UnixNano())
		sb.WriteString("--" + mixedBoundary + "\r\n")
		fmt.Fprintf(sb, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", altBoundary)

		sb.WriteString("--" + altBoundary + "\r\n")
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		sb.WriteString(textBody + "\r\n")
		sb.WriteString("--" + altBoundary + "\r\n")
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		sb.WriteString(htmlBody + "\r\n")
		sb.WriteString("--" + altBoundary + "--\r\n")
		return
	}

	ct := contentType
	if ct == "" {
		ct = "text/plain; charset=UTF-8"
	}
	sb.WriteString("--" + mixedBoundary + "\r\n")
	fmt.Fprintf(sb, "Content-Type: %s\r\n\r\n", ct)
	sb.WriteString(textBody + "\r\n")
}

func encodeBase64Lines(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		sb.WriteString(encoded[i:end])
		if end < len(encoded) {
			sb.WriteString("\r\n")
		}
	}
	return sb.String()
}

func formatHeaders(headers map[string]string) string {
	var lines []string
	for k, v := range headers {
		lines = append(lines, fmt.Sprintf("%s: %s", k, v))
	}
	return strings.Join(lines, "\r\n")
}

func firstNonEmpty(meta map[string]any, keys ...any) string {
	for _, key := range keys {
		switch v := key.(type) {
		case string:
			if meta != nil {
				if _, ok := meta[v]; ok {
					if s := stringValue(meta, v); s != "" {
						return s
					}
				}
			}
		case fmt.Stringer:
			if str := v.String(); strings.TrimSpace(str) != "" {
				return str
			}
		case func() string:
			if str := v(); strings.TrimSpace(str) != "" {
				return str
			}
		default:
			if s := fmt.Sprint(v); strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func stringValue(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	raw, ok := meta[key]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func stripHTML(html string) string {
	// Minimal fallback: drop tags.
	out := strings.Builder{}
	inTag := false
	for _, r := range html {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func htmlToText(html string) string {
	plain, err := html2text.FromString(html, html2text.Options{PrettyTables: true})
	if err == nil {
		if trimmed := strings.TrimSpace(plain); trimmed != "" {
			return trimmed
		}
	}
	return stripHTML(html)
}
