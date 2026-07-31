package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime/multipart"
	"net"
	"net/mail"
	gosmtp "net/smtp"
	"net/textproto"
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
	Host         string
	Port         int
	Username     string
	Password     string
	From         string
	ReplyTo      string
	CC           []string
	BCC          []string
	UseTLS       bool
	UseStartTLS  bool
	TLSPolicy    adapters.TLSPolicy
	Timeout      time.Duration
	LocalName    string
	AuthDisabled bool
	PlainOnly    bool // Force text/plain even when HTML is available.
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
			TLSPolicy:   adapters.TLSPolicyStrict,
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

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) (returnErr error) {
	prepared, err := a.prepareMessage(msg)
	if err != nil {
		return err
	}
	dialer := &net.Dialer{Timeout: a.cfg.Timeout}
	tlsCfg := &tls.Config{ServerName: a.cfg.Host}
	if a.cfg.TLSPolicy == adapters.TLSPolicyInsecureSkipVerify {
		// #nosec G402 -- this is an explicit opt-in compatibility policy;
		// certificate verification remains enabled by default.
		tlsCfg.InsecureSkipVerify = true
	}

	client, conn, clientErr := a.newClient(ctx, dialer, prepared.serverAddress, tlsCfg)
	if clientErr != nil {
		return clientErr
	}
	defer func() {
		if quitErr := client.Quit(); quitErr != nil {
			closeErr := conn.Close()
			if returnErr == nil {
				returnErr = errors.Join(quitErr, closeErr)
			}
		}
	}()

	if err := a.configureClient(client, tlsCfg); err != nil {
		return err
	}
	if err := transmitMessage(client, prepared); err != nil {
		return err
	}

	a.base.LogSuccess(a.name, msg)
	return nil
}

type preparedMessage struct {
	serverAddress string
	from          *mail.Address
	to            *mail.Address
	cc            []*mail.Address
	bcc           []*mail.Address
	data          []byte
}

func (a *Adapter) prepareMessage(msg adapters.Message) (preparedMessage, error) {
	host := strings.TrimSpace(a.cfg.Host)
	if host == "" {
		return preparedMessage{}, fmt.Errorf("smtp: host is required")
	}
	port := a.cfg.Port
	if port == 0 {
		port = 587
	}
	from, err := parseRequiredAddress("from", firstNonEmpty(msg.Metadata, "from", a.cfg.From))
	if err != nil {
		return preparedMessage{}, err
	}
	to, err := parseRequiredAddress("to", msg.To)
	if err != nil {
		return preparedMessage{}, err
	}
	replyTo, err := parseOptionalAddress("reply_to", firstNonEmpty(msg.Metadata, "reply_to", a.cfg.ReplyTo))
	if err != nil {
		return preparedMessage{}, err
	}
	cc, bcc, err := a.parseCopies(msg.Metadata)
	if err != nil {
		return preparedMessage{}, err
	}
	input, err := a.composeInput(msg, from, to, replyTo, cc)
	if err != nil {
		return preparedMessage{}, err
	}
	data, err := composeMessage(input)
	if err != nil {
		return preparedMessage{}, err
	}
	return preparedMessage{
		serverAddress: fmt.Sprintf("%s:%d", host, port),
		from:          from,
		to:            to,
		cc:            cc,
		bcc:           bcc,
		data:          data,
	}, nil
}

func parseRequiredAddress(label, raw string) (*mail.Address, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("smtp: %s address is required", label)
	}
	return parseOptionalAddress(label, raw)
}

func parseOptionalAddress(label, raw string) (*mail.Address, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if err := ensureNoCRLF(raw); err != nil {
		return nil, fmt.Errorf("smtp: invalid %s address: %w", label, err)
	}
	address, err := mail.ParseAddress(raw)
	if err != nil {
		return nil, fmt.Errorf("smtp: invalid %s address: %w", label, err)
	}
	return address, nil
}

func (a *Adapter) parseCopies(metadata map[string]any) ([]*mail.Address, []*mail.Address, error) {
	cc, err := parseAddressList(append(append([]string(nil), a.cfg.CC...), stringSlice(metadata, "cc")...))
	if err != nil {
		return nil, nil, fmt.Errorf("smtp: invalid cc: %w", err)
	}
	bcc, err := parseAddressList(append(append([]string(nil), a.cfg.BCC...), stringSlice(metadata, "bcc")...))
	if err != nil {
		return nil, nil, fmt.Errorf("smtp: invalid bcc: %w", err)
	}
	return cc, bcc, nil
}

func (a *Adapter) composeInput(
	msg adapters.Message,
	from, to, replyTo *mail.Address,
	cc []*mail.Address,
) (composeMessageInput, error) {
	subject := strings.TrimSpace(msg.Subject)
	if err := ensureNoCRLF(subject); err != nil {
		return composeMessageInput{}, fmt.Errorf("smtp: invalid subject: %w", err)
	}
	textBody := firstNonEmpty(msg.Metadata, "text_body", "text", "body", msg.Body)
	htmlBody := firstNonEmpty(msg.Metadata, "html_body", "html")
	if htmlBody != "" && strings.TrimSpace(textBody) == "" {
		textBody = htmlToText(htmlBody)
	}
	contentType := strings.TrimSpace(stringValue(msg.Metadata, "content_type"))
	if contentType == "" {
		contentType = "text/plain; charset=UTF-8"
	}
	if err := ensureNoCRLF(contentType); err != nil {
		return composeMessageInput{}, fmt.Errorf("smtp: invalid content_type: %w", err)
	}
	return composeMessageInput{
		From: from, To: to, Subject: subject, ReplyTo: replyTo, CC: cc,
		TextBody: textBody, HTMLBody: htmlBody, ContentType: contentType,
		PlainOnly: a.cfg.PlainOnly, Attachments: adapters.EmailAttachments(msg.Attachments),
	}, nil
}

func (a *Adapter) configureClient(client *gosmtp.Client, tlsCfg *tls.Config) error {
	if localName := strings.TrimSpace(a.cfg.LocalName); localName != "" {
		if err := client.Hello(localName); err != nil {
			return fmt.Errorf("smtp: hello failed: %w", err)
		}
	}
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
	return nil
}

func transmitMessage(client *gosmtp.Client, prepared preparedMessage) error {
	if err := client.Mail(prepared.from.Address); err != nil {
		return fmt.Errorf("smtp: mail from failed: %w", err)
	}
	for _, recipient := range collectRecipients(prepared.to, prepared.cc, prepared.bcc) {
		if err := client.Rcpt(recipient.Address); err != nil {
			return fmt.Errorf("smtp: rcpt to failed: %w", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: open data: %w", err)
	}
	if _, err := writer.Write(prepared.data); err != nil {
		return errors.Join(fmt.Errorf("smtp: write data: %w", err), writer.Close())
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp: close data: %w", err)
	}
	return nil
}

func (a *Adapter) newClient(ctx context.Context, dialer *net.Dialer, addr string, tlsCfg *tls.Config) (*gosmtp.Client, net.Conn, error) {
	if a.cfg.UseTLS {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: tlsCfg}
		conn, err := tlsDialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, nil, fmt.Errorf("smtp: tls dial failed: %w", err)
		}
		client, err := gosmtp.NewClient(conn, a.cfg.Host)
		if err != nil {
			closeErr := conn.Close()
			return nil, nil, errors.Join(fmt.Errorf("smtp: new client failed: %w", err), closeErr)
		}
		return client, conn, nil
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("smtp: dial failed: %w", err)
	}
	client, err := gosmtp.NewClient(conn, a.cfg.Host)
	if err != nil {
		closeErr := conn.Close()
		return nil, nil, errors.Join(fmt.Errorf("smtp: new client failed: %w", err), closeErr)
	}
	return client, conn, nil
}

type composeMessageInput struct {
	From        *mail.Address
	To          *mail.Address
	Subject     string
	ReplyTo     *mail.Address
	CC          []*mail.Address
	TextBody    string
	HTMLBody    string
	ContentType string
	PlainOnly   bool
	Attachments []adapters.Attachment
}

func composeMessage(input composeMessageInput) ([]byte, error) {
	if input.From == nil || input.To == nil {
		return nil, fmt.Errorf("smtp: from and to are required")
	}
	subject, err := sanitizeHeaderValue(input.Subject)
	if err != nil {
		return nil, fmt.Errorf("smtp: invalid subject: %w", err)
	}
	contentType, err := sanitizeHeaderValue(input.ContentType)
	if err != nil {
		return nil, fmt.Errorf("smtp: invalid content_type: %w", err)
	}
	if contentType == "" {
		contentType = "text/plain; charset=UTF-8"
	}

	textBody := input.TextBody
	if strings.TrimSpace(textBody) == "" && strings.TrimSpace(input.HTMLBody) != "" {
		textBody = htmlToText(input.HTMLBody)
	}

	body, bodyContentType, err := buildBody(textBody, input.HTMLBody, contentType, input.PlainOnly, adapters.EmailAttachments(input.Attachments))
	if err != nil {
		return nil, err
	}

	var msg bytes.Buffer
	writeHeader(&msg, "From", input.From.String())
	writeHeader(&msg, "To", input.To.String())
	if len(input.CC) > 0 {
		writeHeader(&msg, "Cc", formatAddressList(input.CC))
	}
	if input.ReplyTo != nil {
		writeHeader(&msg, "Reply-To", input.ReplyTo.String())
	}
	if subject != "" {
		writeHeader(&msg, "Subject", subject)
	}
	writeHeader(&msg, "MIME-Version", "1.0")
	writeHeader(&msg, "Content-Type", bodyContentType)
	msg.WriteString("\r\n")
	msg.Write(body)
	return msg.Bytes(), nil
}

func buildBody(textBody, htmlBody, contentType string, plainOnly bool, attachments []adapters.Attachment) ([]byte, string, error) {
	if len(attachments) > 0 {
		return buildMixedBody(textBody, htmlBody, contentType, plainOnly, attachments)
	}
	if !plainOnly && strings.TrimSpace(htmlBody) != "" {
		return buildAlternativeBody(textBody, htmlBody)
	}
	if contentType == "" {
		contentType = "text/plain; charset=UTF-8"
	}
	return []byte(textBody), contentType, nil
}

func buildMixedBody(textBody, htmlBody, contentType string, plainOnly bool, attachments []adapters.Attachment) ([]byte, string, error) {
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)

	if err := writeBodyPart(writer, textBody, htmlBody, contentType, plainOnly); err != nil {
		return nil, "", err
	}
	for _, att := range attachments {
		if err := writeAttachmentPart(writer, att); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("smtp: close mixed body: %w", err)
	}
	return out.Bytes(), fmt.Sprintf("multipart/mixed; boundary=%s", writer.Boundary()), nil
}

func writeBodyPart(writer *multipart.Writer, textBody, htmlBody, contentType string, plainOnly bool) error {
	if plainOnly || strings.TrimSpace(htmlBody) == "" {
		if contentType == "" {
			contentType = "text/plain; charset=UTF-8"
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return fmt.Errorf("smtp: create body part: %w", err)
		}
		_, err = part.Write([]byte(textBody))
		if err != nil {
			return fmt.Errorf("smtp: write body part: %w", err)
		}
		return nil
	}

	if strings.TrimSpace(textBody) == "" {
		textBody = htmlToText(htmlBody)
	}
	boundary := fmt.Sprintf("alt-%d", time.Now().UnixNano())
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%s", boundary))
	altPart, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("smtp: create alternative container: %w", err)
	}
	altWriter := multipart.NewWriter(altPart)
	if boundaryErr := altWriter.SetBoundary(boundary); boundaryErr != nil {
		return fmt.Errorf("smtp: set alternative boundary: %w", boundaryErr)
	}

	plainHeader := textproto.MIMEHeader{}
	plainHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	plainPart, err := altWriter.CreatePart(plainHeader)
	if err != nil {
		return fmt.Errorf("smtp: create text part: %w", err)
	}
	if _, writeErr := plainPart.Write([]byte(textBody)); writeErr != nil {
		return fmt.Errorf("smtp: write text part: %w", writeErr)
	}

	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlPart, err := altWriter.CreatePart(htmlHeader)
	if err != nil {
		return fmt.Errorf("smtp: create html part: %w", err)
	}
	if _, err := htmlPart.Write([]byte(htmlBody)); err != nil {
		return fmt.Errorf("smtp: write html part: %w", err)
	}

	if err := altWriter.Close(); err != nil {
		return fmt.Errorf("smtp: close alternative writer: %w", err)
	}
	return nil
}

func buildAlternativeBody(textBody, htmlBody string) ([]byte, string, error) {
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)

	plainHeader := textproto.MIMEHeader{}
	plainHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	plainPart, err := writer.CreatePart(plainHeader)
	if err != nil {
		return nil, "", fmt.Errorf("smtp: create text part: %w", err)
	}
	if _, writeErr := plainPart.Write([]byte(textBody)); writeErr != nil {
		return nil, "", fmt.Errorf("smtp: write text part: %w", writeErr)
	}

	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlPart, err := writer.CreatePart(htmlHeader)
	if err != nil {
		return nil, "", fmt.Errorf("smtp: create html part: %w", err)
	}
	if _, err := htmlPart.Write([]byte(htmlBody)); err != nil {
		return nil, "", fmt.Errorf("smtp: write html part: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("smtp: close alternative body: %w", err)
	}
	return out.Bytes(), fmt.Sprintf("multipart/alternative; boundary=%s", writer.Boundary()), nil
}

func writeAttachmentPart(writer *multipart.Writer, attachment adapters.Attachment) error {
	filename, err := sanitizeFilename(attachment.Filename)
	if err != nil {
		return fmt.Errorf("smtp: invalid attachment filename: %w", err)
	}
	contentType := strings.TrimSpace(attachment.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if validationErr := ensureNoCRLF(contentType); validationErr != nil {
		return fmt.Errorf("smtp: invalid attachment content_type: %w", validationErr)
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Type", contentType)
	header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	header.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("smtp: create attachment part: %w", err)
	}
	_, err = part.Write([]byte(encodeBase64Lines(attachment.Content)))
	if err != nil {
		return fmt.Errorf("smtp: write attachment part: %w", err)
	}
	return nil
}

func collectRecipients(to *mail.Address, cc, bcc []*mail.Address) []*mail.Address {
	out := []*mail.Address{to}
	out = append(out, cc...)
	out = append(out, bcc...)
	return out
}

func parseAddressList(values []string) ([]*mail.Address, error) {
	out := make([]*mail.Address, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if err := ensureNoCRLF(trimmed); err != nil {
			return nil, err
		}
		addr, err := mail.ParseAddress(trimmed)
		if err != nil {
			return nil, err
		}
		out = append(out, addr)
	}
	return out, nil
}

func formatAddressList(addresses []*mail.Address) string {
	if len(addresses) == 0 {
		return ""
	}
	parts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if addr == nil {
			continue
		}
		parts = append(parts, addr.String())
	}
	return strings.Join(parts, ", ")
}

func writeHeader(buf *bytes.Buffer, key, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

func sanitizeFilename(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "attachment", nil
	}
	if err := ensureNoCRLF(trimmed); err != nil {
		return "", err
	}
	if strings.ContainsAny(trimmed, `"\\`) {
		return "", fmt.Errorf("filename contains unsafe characters")
	}
	return trimmed, nil
}

func sanitizeHeaderValue(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if err := ensureNoCRLF(trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

func ensureNoCRLF(value string) error {
	if strings.Contains(value, "\r") || strings.Contains(value, "\n") {
		return fmt.Errorf("value contains CR/LF")
	}
	return nil
}

func encodeBase64Lines(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := min(i+76, len(encoded))
		sb.WriteString(encoded[i:end])
		if end < len(encoded) {
			sb.WriteString("\r\n")
		}
	}
	return sb.String()
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

func stringSlice(meta map[string]any, key string) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			str := strings.TrimSpace(fmt.Sprint(entry))
			if str == "" {
				continue
			}
			out = append(out, str)
		}
		return out
	default:
		return nil
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
