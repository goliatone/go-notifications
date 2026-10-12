package slack

import (
	"context"
	"fmt"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStructuredReplyAndLegacySender(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		fmt.Fprint(w, `{"ok":true,"channel":"C","ts":"2.000001","message":{"thread_ts":"1.000001"}}`)
	}))
	defer server.Close()
	adapter := New(&logger.Nop{}, WithConfig(Config{Token: "fixture", Channel: "C", BaseURL: server.URL}), WithClient(server.Client()))
	message := adapters.Message{Body: "Reply", Metadata: map[string]any{"thread_ts": "1.000001"}}
	result, err := adapter.SendWithResult(context.Background(), message)
	if err != nil || result.State != "sent" || result.MessageID != "2.000001" || result.ThreadID != "1.000001" {
		t.Fatalf("structured: %+v %v", result, err)
	}
	if err := adapter.Send(context.Background(), message); err != nil || count != 2 {
		t.Fatalf("legacy: %v count=%d", err, count)
	}
}
