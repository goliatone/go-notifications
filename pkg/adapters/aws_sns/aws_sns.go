package aws_sns

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter delivers messages via Amazon SNS (SMS/email topics).
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client *http.Client
}

// Config holds SNS settings.
type Config struct {
	Region       string
	AccessKey    string
	SecretKey    string
	SessionToken string
	TopicARN     string // optional; can be overridden per-message with metadata["topic_arn"]
	DryRun       bool
	Timeout      time.Duration
}

type Option func(*Adapter)

// WithName overrides adapter name.
func WithName(name string) Option {
	return func(a *Adapter) {
		if strings.TrimSpace(name) != "" {
			a.name = name
		}
	}
}

// WithConfig sets SNS configuration.
func WithConfig(cfg Config) Option {
	return func(a *Adapter) {
		a.cfg = cfg
	}
}

// WithHTTPClient injects a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

// New constructs the SNS adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "aws_sns",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "aws_sns",
			Channels: []string{"sms", "chat"}, // topic-based fanout; use sms or chat logical channels.
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			Region:  "us-east-1",
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	if adapter.client == nil {
		adapter.client = &http.Client{Timeout: adapter.cfg.Timeout}
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	if a.cfg.DryRun {
		a.base.LogSuccess(a.name, msg)
		a.base.Logger().Info("[aws_sns:during-dry-run] send skipped",
			logger.Field{Key: "to", Value: msg.To},
			logger.Field{Key: "channel", Value: msg.Channel},
			logger.Field{Key: "subject", Value: msg.Subject},
		)
		return nil
	}
	body := firstNonEmpty(stringValue(msg.Metadata, "body"), msg.Body)
	htmlBody := firstNonEmpty(stringValue(msg.Metadata, "html_body"))
	if htmlBody != "" && body == "" {
		// SNS SMS does not support HTML; topics can fan out to email subscribers that can use HTML.
		body = stripHTML(htmlBody)
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("aws_sns: message body required")
	}

	// Determine destination: PhoneNumber for SMS direct, TopicArn for topic fanout
	topicARN := firstNonEmpty(stringValue(msg.Metadata, "topic_arn"), a.cfg.TopicARN)
	params := url.Values{}
	params.Set("Action", "Publish")
	params.Set("Message", body)
	if subj := strings.TrimSpace(msg.Subject); subj != "" {
		params.Set("Subject", subj)
	}
	if topicARN != "" {
		params.Set("TopicArn", topicARN)
	} else {
		to := strings.TrimSpace(msg.To)
		if to == "" {
			return fmt.Errorf("aws_sns: topic_arn or destination required")
		}
		params.Set("PhoneNumber", to)
	}

	creds := a.loadCredentials()
	if creds.AccessKey == "" || creds.SecretKey == "" {
		return fmt.Errorf("aws_sns: aws credentials required")
	}
	region := strings.TrimSpace(a.cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	req, signedHeaders, err := a.signRequest(creds, region, params)
	if err != nil {
		return err
	}
	for k, v := range signedHeaders {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("aws_sns: request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("aws_sns: unexpected status %d", resp.StatusCode)
	}
	a.base.LogSuccess(a.name, msg)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
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

type credentials struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
}

func (a *Adapter) loadCredentials() credentials {
	creds := credentials{
		AccessKey:    strings.TrimSpace(a.cfg.AccessKey),
		SecretKey:    strings.TrimSpace(a.cfg.SecretKey),
		SessionToken: strings.TrimSpace(a.cfg.SessionToken),
	}
	if creds.AccessKey == "" {
		creds.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	if creds.SecretKey == "" {
		creds.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	if creds.SessionToken == "" {
		creds.SessionToken = os.Getenv("AWS_SESSION_TOKEN")
	}
	return creds
}

func (a *Adapter) signRequest(creds credentials, region string, params url.Values) (*http.Request, map[string]string, error) {
	endpoint := fmt.Sprintf("https://sns.%s.amazonaws.com/", region)
	bodyStr := params.Encode()
	payloadHash := sha256Hex([]byte(bodyStr))

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")

	canonicalURI := "/"
	canonicalQuery := ""
	canonicalHeaders := fmt.Sprintf("content-type:application/x-www-form-urlencoded; charset=utf-8\nhost:sns.%s.amazonaws.com\nx-amz-date:%s\n", region, amzDate)
	if creds.SessionToken != "" {
		canonicalHeaders += fmt.Sprintf("x-amz-security-token:%s\n", creds.SessionToken)
	}
	signedHeaders := "content-type;host;x-amz-date"
	if creds.SessionToken != "" {
		signedHeaders += ";x-amz-security-token"
	}

	canonicalRequest := strings.Join([]string{
		"POST",
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/sns/aws4_request", date, region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveKey(creds.SecretKey, date, region, "sns")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKey, credentialScope, signedHeaders, signature)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(bodyStr))
	if err != nil {
		return nil, nil, err
	}
	headers := map[string]string{
		"Content-Type":  "application/x-www-form-urlencoded; charset=utf-8",
		"X-Amz-Date":    amzDate,
		"Authorization": authHeader,
	}
	if creds.SessionToken != "" {
		headers["X-Amz-Security-Token"] = creds.SessionToken
	}
	return req, headers, nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func deriveKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}
func stripHTML(html string) string {
	var b strings.Builder
	inTag := false
	for _, r := range html {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}
