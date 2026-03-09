package adapters_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ses"
	core "github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/adapters/aws_ses"
	"github.com/goliatone/go-notifications/pkg/adapters/aws_sns"
	"github.com/goliatone/go-notifications/pkg/adapters/firebase"
	"github.com/goliatone/go-notifications/pkg/adapters/slack"
	"github.com/goliatone/go-notifications/pkg/adapters/telegram"
	"github.com/goliatone/go-notifications/pkg/adapters/twilio"
	"github.com/goliatone/go-notifications/pkg/adapters/webhook"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

type countingRoundTripper struct {
	calls int
}

func (c *countingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls++
	return nil, errors.New("unexpected network call")
}

func newCountingHTTPClient(rt *countingRoundTripper) *http.Client {
	return &http.Client{Transport: rt}
}

func TestDryRunContractSkipsCredentialGatingAndNetworkIO(t *testing.T) {
	type testCase struct {
		name string
		run  func(ctx context.Context, rt *countingRoundTripper) error
	}

	tests := []testCase{
		{
			name: "twilio",
			run: func(ctx context.Context, rt *countingRoundTripper) error {
				a := twilio.New(&logger.Nop{},
					twilio.WithConfig(twilio.Config{DryRun: true}),
					twilio.WithClient(newCountingHTTPClient(rt)),
				)
				return a.Send(ctx, core.Message{Channel: "sms", To: "+15551234567", Body: "hello"})
			},
		},
		{
			name: "telegram",
			run: func(ctx context.Context, rt *countingRoundTripper) error {
				a := telegram.New(&logger.Nop{},
					telegram.WithConfig(telegram.Config{DryRun: true}),
					telegram.WithClient(newCountingHTTPClient(rt)),
				)
				return a.Send(ctx, core.Message{Channel: "chat", To: "12345", Body: "hello"})
			},
		},
		{
			name: "slack",
			run: func(ctx context.Context, rt *countingRoundTripper) error {
				a := slack.New(&logger.Nop{},
					slack.WithConfig(slack.Config{DryRun: true, Channel: "alerts"}),
					slack.WithClient(newCountingHTTPClient(rt)),
				)
				return a.Send(ctx, core.Message{Channel: "chat", Body: "hello"})
			},
		},
		{
			name: "webhook",
			run: func(ctx context.Context, rt *countingRoundTripper) error {
				a := webhook.New(&logger.Nop{},
					webhook.WithConfig(webhook.Config{DryRun: true}),
					webhook.WithClient(newCountingHTTPClient(rt)),
				)
				return a.Send(ctx, core.Message{Channel: "webhook", Body: "hello"})
			},
		},
		{
			name: "firebase",
			run: func(ctx context.Context, rt *countingRoundTripper) error {
				a := firebase.New(&logger.Nop{},
					firebase.WithConfig(firebase.Config{DryRun: true}),
					firebase.WithClient(newCountingHTTPClient(rt)),
				)
				return a.Send(ctx, core.Message{Channel: "push", To: "device-token", Body: "hello"})
			},
		},
		{
			name: "aws_sns",
			run: func(ctx context.Context, rt *countingRoundTripper) error {
				a := aws_sns.New(&logger.Nop{},
					aws_sns.WithConfig(aws_sns.Config{DryRun: true}),
					aws_sns.WithHTTPClient(newCountingHTTPClient(rt)),
				)
				return a.Send(ctx, core.Message{Channel: "sms", To: "+15551234567", Body: "hello"})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &countingRoundTripper{}
			err := tc.run(context.Background(), rt)
			if err != nil {
				t.Fatalf("expected dry-run success, got %v", err)
			}
			if rt.calls != 0 {
				t.Fatalf("expected no network calls during dry-run, got %d", rt.calls)
			}
		})
	}
}

type fakeSESClient struct {
	called bool
}

func (c *fakeSESClient) SendEmail(context.Context, *ses.SendEmailInput, ...func(*ses.Options)) (*ses.SendEmailOutput, error) {
	c.called = true
	return &ses.SendEmailOutput{}, nil
}

func TestSESDryRunSkipsClientCall(t *testing.T) {
	client := &fakeSESClient{}
	a := aws_ses.New(&logger.Nop{},
		aws_ses.WithConfig(aws_ses.Config{DryRun: true, From: "noreply@example.com"}),
		aws_ses.WithClient(client),
	)
	err := a.Send(context.Background(), core.Message{Channel: "email", To: "user@example.com", Body: "hello"})
	if err != nil {
		t.Fatalf("expected dry-run success, got %v", err)
	}
	if client.called {
		t.Fatalf("expected no SES client call during dry-run")
	}
}

func TestPayloadEncodeFailuresAreSurfaced(t *testing.T) {
	t.Run("webhook", func(t *testing.T) {
		a := webhook.New(&logger.Nop{}, webhook.WithConfig(webhook.Config{
			URL:             "https://example.com/webhook",
			Method:          "POST",
			ForwardMetadata: true,
		}))
		err := a.Send(context.Background(), core.Message{
			Channel: "webhook",
			Body:    "hello",
			Metadata: map[string]any{
				"invalid": make(chan int),
			},
		})
		if err == nil {
			t.Fatalf("expected payload encoding error")
		}
		if !errors.Is(err, core.ErrPayloadEncode) {
			t.Fatalf("expected ErrPayloadEncode, got %v", err)
		}
	})

	t.Run("firebase", func(t *testing.T) {
		a := firebase.New(&logger.Nop{}, firebase.WithConfig(firebase.Config{
			ServerKey: "key",
			Endpoint:  "https://fcm.googleapis.com/fcm/send",
		}))
		err := a.Send(context.Background(), core.Message{
			Channel: "push",
			To:      "device-token",
			Body:    "hello",
			Metadata: map[string]any{
				"data": map[string]any{"invalid": make(chan int)},
			},
		})
		if err == nil {
			t.Fatalf("expected payload encoding error")
		}
		if !errors.Is(err, core.ErrPayloadEncode) {
			t.Fatalf("expected ErrPayloadEncode, got %v", err)
		}
	})
}
