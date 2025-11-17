package broadcaster

import (
	"context"
	"errors"
	"testing"
)

func TestFanoutBroadcast(t *testing.T) {
	var received []Event
	fn := Func(func(ctx context.Context, evt Event) error {
		received = append(received, evt)
		return nil
	})
	f := NewFanout(fn, fn)
	if err := f.Broadcast(context.Background(), Event{Topic: "inbox", Payload: "hello"}); err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("expected event fanout, got %d", len(received))
	}
}

func TestFanoutReturnsFirstError(t *testing.T) {
	calls := 0
	errExpected := errors.New("boom")
	fn := Func(func(ctx context.Context, evt Event) error {
		calls++
		if calls == 1 {
			return errExpected
		}
		return nil
	})
	f := NewFanout(fn, fn)
	err := f.Broadcast(context.Background(), Event{})
	if !errors.Is(err, errExpected) {
		t.Fatalf("expected error %v, got %v", errExpected, err)
	}
	if calls != 2 {
		t.Fatalf("expected both sinks invoked, got %d", calls)
	}
}
