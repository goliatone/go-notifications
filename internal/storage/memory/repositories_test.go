package memory

import (
	"context"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
)

func TestDigestMembershipCannotAttachToClaimedWindow(t *testing.T) { //nolint:gocyclo // Linear state-transition contract.
	ctx := context.Background()
	events := NewEventRepository()
	publications := NewPublicationRepository(events)
	open := &domain.NotificationPublication{
		Kind: "digest", DigestKey: "digest-a", QueueKey: "publication-open",
		Status: domain.PublicationStatusPending,
	}
	if err := publications.Create(ctx, open); err != nil {
		t.Fatalf("create open publication: %v", err)
	}
	first := &domain.NotificationEvent{
		DefinitionCode: "notice", Recipients: domain.StringList{"subject-1"},
	}
	if err := events.Create(ctx, first); err != nil {
		t.Fatalf("create first event: %v", err)
	}
	joined, created, err := publications.CreateOrAttachOpenDigest(ctx, &domain.NotificationPublication{
		Kind: "digest", DigestKey: open.DigestKey, QueueKey: "candidate-first",
		Status: domain.PublicationStatusPending,
	}, first)
	if err != nil || created || joined.ID != open.ID {
		t.Fatalf("first member did not join open window: joined=%+v created=%v err=%v", joined, created, err)
	}
	claimed, err := publications.Claim(ctx, open.ID, time.Now().Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim publication: claimed=%v err=%v", claimed, err)
	}
	members, err := events.ListByPublication(ctx, open.ID)
	if err != nil || len(members) != 1 || members[0].ID != first.ID {
		t.Fatalf("claimed window did not contain its acknowledged member: members=%+v err=%v", members, err)
	}

	late := &domain.NotificationEvent{
		DefinitionCode: "notice", Recipients: domain.StringList{"subject-2"},
	}
	if createErr := events.Create(ctx, late); createErr != nil {
		t.Fatalf("create late event: %v", createErr)
	}
	nextCandidate := &domain.NotificationPublication{
		Kind: "digest", DigestKey: open.DigestKey, QueueKey: "candidate-next",
		Status: domain.PublicationStatusPending,
	}
	next, created, err := publications.CreateOrAttachOpenDigest(ctx, nextCandidate, late)
	if err != nil || !created || next.ID == open.ID {
		t.Fatalf("late member was not moved to a new window: next=%+v created=%v err=%v", next, created, err)
	}
	nextMembers, err := events.ListByPublication(ctx, next.ID)
	if err != nil || len(nextMembers) != 1 || nextMembers[0].ID != late.ID {
		t.Fatalf("new window did not contain late member: members=%+v err=%v", nextMembers, err)
	}
}

func TestDefinitionRepositoryMemory(t *testing.T) {
	repo := NewDefinitionRepository()
	ctx := context.Background()

	def := &domain.NotificationDefinition{
		Code:     "welcome",
		Name:     "Welcome",
		Channels: domain.StringList{"email"},
	}
	if err := repo.Create(ctx, def); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByCode(ctx, "welcome")
	if err != nil {
		t.Fatalf("get by code: %v", err)
	}
	if got.Code != "welcome" {
		t.Fatalf("expected code welcome, got %s", got.Code)
	}

	result, err := repo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected total 1, got %d", result.Total)
	}
}
