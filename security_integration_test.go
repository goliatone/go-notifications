package notifications_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	i18n "github.com/goliatone/go-i18n"
	notifications "github.com/goliatone/go-notifications"
	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/definitions"
	"github.com/goliatone/go-notifications/pkg/deliveries"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/links"
	"github.com/goliatone/go-notifications/pkg/notifier"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/receipts"
	"github.com/goliatone/go-notifications/pkg/retention"
	"github.com/goliatone/go-notifications/pkg/storage"
	"github.com/goliatone/go-notifications/pkg/templates"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

const (
	sensitiveValue       = "https://credentials.example.test/one-time/secret-123"
	resolvedDestination  = "private-recipient@example.test"
	failureDestination   = "failure-recipient@example.test"
	scheduledDestination = "scheduled-recipient@example.test"
	rawProviderErrorText = "provider rejected private-recipient@example.test secret-123"
)

func TestSensitiveNotificationContractAcrossProviders(t *testing.T) {
	providers := []struct {
		name  string
		setup func(*testing.T) (storage.Providers, func())
	}{
		{name: "memory", setup: setupMemorySecurityProvider},
		{name: "sqlite", setup: setupSQLiteSecurityProvider},
		{name: "postgres", setup: setupPostgresSecurityProvider},
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			repositories, cleanup := provider.setup(t)
			defer cleanup()
			runSensitiveNotificationContract(t, repositories)
		})
	}
}

func TestScopedIdempotencyIsAtomicAcrossProviders(t *testing.T) {
	for _, provider := range securityProviderFactories() {
		t.Run(provider.name, func(t *testing.T) {
			repositories, cleanup := provider.setup(t)
			defer cleanup()
			ctx := context.Background()
			const callers = 16
			var wg sync.WaitGroup
			var mu sync.Mutex
			createdCount := 0
			ids := make(map[uuid.UUID]struct{})
			errs := make(chan error, callers)
			for range callers {
				wg.Go(func() {
					stored, created, err := repositories.Events.CreateIdempotent(ctx, &domain.NotificationEvent{
						DefinitionCode:   "concurrent.notice",
						IdempotencyScope: "tenant:one", IdempotencyKey: "same-key",
						RequestFingerprint: "same-fingerprint",
					})
					if err != nil {
						errs <- err
						return
					}
					mu.Lock()
					defer mu.Unlock()
					if created {
						createdCount++
					}
					ids[stored.ID] = struct{}{}
				})
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Errorf("concurrent idempotency: %v", err)
			}
			if createdCount != 1 || len(ids) != 1 {
				t.Fatalf("idempotency was not atomic: created=%d ids=%v", createdCount, ids)
			}
		})
	}
}

func TestDigestMembershipBoundaryAcrossProviders(t *testing.T) { //nolint:gocyclo // Same state contract across providers.
	for _, provider := range securityProviderFactories() {
		t.Run(provider.name, func(t *testing.T) {
			repositories, cleanup := provider.setup(t)
			defer cleanup()
			ctx := context.Background()
			digestKey := "digest-" + uuid.NewString()
			open := &domain.NotificationPublication{
				Kind: "digest", DigestKey: digestKey,
				QueueKey: "notification-publication:" + uuid.NewString(),
				Status:   domain.PublicationStatusPending,
			}
			if err := repositories.Publications.Create(ctx, open); err != nil {
				t.Fatalf("create open publication: %v", err)
			}
			first := &domain.NotificationEvent{
				DefinitionCode: "digest.notice", Recipients: domain.StringList{"subject-1"},
			}
			if err := repositories.Events.Create(ctx, first); err != nil {
				t.Fatalf("create first event: %v", err)
			}
			joined, created, err := repositories.Publications.CreateOrAttachOpenDigest(
				ctx,
				&domain.NotificationPublication{
					Kind: "digest", DigestKey: digestKey,
					QueueKey: "notification-publication:" + uuid.NewString(),
					Status:   domain.PublicationStatusPending,
				},
				first,
			)
			if err != nil || created || joined.ID != open.ID {
				t.Fatalf("join open publication: joined=%+v created=%v err=%v", joined, created, err)
			}
			claimed, err := repositories.Publications.Claim(ctx, open.ID, time.Now().Add(time.Minute))
			if err != nil || !claimed {
				t.Fatalf("claim open publication: claimed=%v err=%v", claimed, err)
			}
			late := &domain.NotificationEvent{
				DefinitionCode: "digest.notice", Recipients: domain.StringList{"subject-2"},
			}
			if createErr := repositories.Events.Create(ctx, late); createErr != nil {
				t.Fatalf("create late event: %v", createErr)
			}
			next, created, err := repositories.Publications.CreateOrAttachOpenDigest(
				ctx,
				&domain.NotificationPublication{
					Kind: "digest", DigestKey: digestKey,
					QueueKey: "notification-publication:" + uuid.NewString(),
					Status:   domain.PublicationStatusPending,
				},
				late,
			)
			if err != nil || !created || next.ID == open.ID {
				t.Fatalf("late join did not create next window: next=%+v created=%v err=%v", next, created, err)
			}
			oldMembers, err := repositories.Events.ListByPublication(ctx, open.ID)
			if err != nil || len(oldMembers) != 1 || oldMembers[0].ID != first.ID {
				t.Fatalf("claimed window membership changed: members=%+v err=%v", oldMembers, err)
			}
			nextMembers, err := repositories.Events.ListByPublication(ctx, next.ID)
			if err != nil || len(nextMembers) != 1 || nextMembers[0].ID != late.ID {
				t.Fatalf("next window lost late member: members=%+v err=%v", nextMembers, err)
			}
		})
	}
}

type securityProviderFactory struct {
	name  string
	setup func(*testing.T) (storage.Providers, func())
}

func securityProviderFactories() []securityProviderFactory {
	return []securityProviderFactory{
		{name: "memory", setup: setupMemorySecurityProvider},
		{name: "sqlite", setup: setupSQLiteSecurityProvider},
		{name: "postgres", setup: setupPostgresSecurityProvider},
	}
}

// The intentionally linear scenario keeps every leak assertion in execution
// order so provider failures identify the exact boundary that regressed.
func runSensitiveNotificationContract(t *testing.T, repositories storage.Providers) { //nolint:gocyclo,funlen // Ordered end-to-end leak audit.
	t.Helper()
	ctx := context.Background()
	adapter := &securityAdapter{}
	resolver := &securityResolver{}
	q := &securityQueue{}
	activitySink := &securityActivity{}
	broadcastSink := &securityBroadcaster{}
	logSink := &securityLogger{}
	linkBuilder := &securityLinkBuilder{}
	linkStore := &securityLinkStore{}
	linkObserver := &securityLinkObserver{}
	diagnostic := &securityDiagnostic{}
	cfg := config.Defaults()
	cfg.Dispatcher.EnvFallbackAllowlist = []string{
		"subject-sensitive", "subject-failure", "subject-scheduled", "subject-retry",
	}

	module, err := notifier.NewModule(notifier.ModuleOptions{
		Config: cfg, Storage: repositories, Translator: securityTranslator(t),
		Logger: logSink, Queue: q, Broadcaster: broadcastSink,
		Adapters: []adapters.Messenger{adapter}, RecipientResolver: resolver,
		LinkBuilder: linkBuilder, LinkStore: linkStore, LinkObserver: linkObserver,
		Activity: activity.Hooks{activitySink}, Privacy: privacy.DefaultPolicy{},
		Diagnostic: diagnostic,
	})
	if err != nil {
		t.Fatalf("construct module: %v", err)
	}
	seedSecurityDefinitions(t, ctx, module)

	request := events.ImmediateRequest{
		DefinitionCode: "sensitive", Recipients: []string{"subject-sensitive"},
		Context:        map[string]any{"safe_label": "welcome"},
		Transient:      map[string]any{"credential_url": sensitiveValue},
		Locale:         "en",
		IdempotencyKey: "sensitive-once", CorrelationID: "correlation-safe",
	}
	first, err := module.Events().DispatchImmediate(ctx, request)
	if err != nil || first.Status != receipts.StatusProcessed {
		t.Fatalf("sensitive delivery: receipt=%+v err=%v", first, err)
	}
	replay, err := module.Events().DispatchImmediate(ctx, request)
	if err != nil || !replay.Replay || replay.EventID != first.EventID || adapter.count() != 1 {
		t.Fatalf("sensitive replay: receipt=%+v sends=%d err=%v", replay, adapter.count(), err)
	}
	if !adapter.contains(sensitiveValue) || !adapter.destinationSeen(resolvedDestination) {
		t.Fatalf("adapter did not receive transient render data at resolved destination")
	}
	sendsBeforeLookup := adapter.count()
	recovered, err := module.LookupReceipt(ctx, events.ReceiptLookup{
		DefinitionCode: "sensitive", IdempotencyScope: "system", IdempotencyKey: "sensitive-once",
	})
	if err != nil || recovered.EventID != first.EventID || !recovered.Replay || adapter.count() != sendsBeforeLookup {
		t.Fatalf("side-effect-free receipt recovery: receipt=%+v sends=%d err=%v", recovered, adapter.count(), err)
	}
	inspection, err := module.Deliveries().List(ctx, deliveries.ListQuery{
		Scope: "system", DefinitionCode: "sensitive", Limit: 100,
	})
	if err != nil || len(inspection.Items) == 0 {
		t.Fatalf("safe delivery inspection: page=%+v err=%v", inspection, err)
	}
	if containsSecret(inspection) || strings.Contains(fmt.Sprint(inspection), "subject-sensitive") {
		t.Fatalf("delivery inspection leaked sensitive state: %+v", inspection)
	}
	if !linkBuilder.contains(sensitiveValue) {
		t.Fatalf("link builder did not receive transient render data")
	}
	if linkStore.count() != 0 || linkObserver.count() != 0 {
		t.Fatalf("transient link projections escaped: stores=%d observers=%d", linkStore.count(), linkObserver.count())
	}

	event, err := repositories.Events.GetByID(ctx, first.EventID)
	if err != nil || !event.TransientDependent {
		t.Fatalf("transient dependency marker missing: event=%+v err=%v", event, err)
	}
	if event.Locale != "en" || len(event.Channels) != 2 {
		t.Fatalf("durable event delivery plan missing: %+v", event)
	}
	if containsSecret(event.Context) || containsSecret(event.Recipients) {
		t.Fatalf("event persisted sensitive content: %+v", event)
	}
	messages, err := repositories.Messages.ListByEvent(ctx, first.EventID)
	if err != nil || len(messages) != 2 {
		t.Fatalf("list sensitive messages: count=%d err=%v", len(messages), err)
	}
	var deliveredMessageID uuid.UUID
	for _, message := range messages {
		if message.Receiver != "subject-sensitive" || message.Subject != "" ||
			message.Body != "" || containsSecret(message) {
			t.Fatalf("metadata-only message leaked or lost opaque identity: %+v", message)
		}
		if message.Channel == "email" {
			deliveredMessageID = message.ID
			if message.TemplateCode == "" ||
				len(message.ProviderPlan) != 1 || message.ProviderPlan[0] != "security" {
				t.Fatalf("durable message delivery plan missing: %+v", message)
			}
		}
	}
	if deliveredMessageID == uuid.Nil {
		t.Fatalf("email message state was not persisted")
	}
	attempts, err := repositories.DeliveryAttempts.ListByMessage(ctx, deliveredMessageID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if containsSecret(attempts) {
		t.Fatalf("attempts leaked sensitive content: %+v", attempts)
	}
	inboxItems, err := repositories.Inbox.ListByUser(ctx, "subject-sensitive", store.ListOptions{})
	if err != nil || inboxItems.Total != 0 || broadcastSink.count() != 0 {
		t.Fatalf("transient inbox projection escaped: inbox=%d broadcasts=%d err=%v", inboxItems.Total, broadcastSink.count(), err)
	}
	if containsSecret(first) || containsSecret(replay) || activitySink.containsSecret() || logSink.containsSecret() {
		t.Fatalf("safe observability leaked transient content")
	}
	retryReceipt, retryErr := module.RetryWithReceipt(ctx, events.RetryRequest{
		EventID: first.EventID, IdempotencyKey: "unsafe-retry",
	})
	var retrySafe privacy.SafeError
	if !errors.As(retryErr, &retrySafe) || retrySafe.Code != "transient_retry_forbidden" ||
		retryReceipt.Status != receipts.StatusFailed {
		t.Fatalf("transient retry was not rejected safely: receipt=%+v err=%#v", retryReceipt, retryErr)
	}

	adapter.setError(errors.New(rawProviderErrorText))
	failed, failureErr := module.Events().DispatchImmediate(ctx, events.ImmediateRequest{
		DefinitionCode: "sensitive", Recipients: []string{"subject-failure"},
		Transient: map[string]any{"credential_url": sensitiveValue},
	})
	var failureSafe privacy.SafeError
	if !errors.As(failureErr, &failureSafe) || failed.Status != receipts.StatusFailed ||
		strings.Contains(failureErr.Error(), "secret-123") || strings.Contains(failureErr.Error(), resolvedDestination) {
		t.Fatalf("provider failure was not sanitized: receipt=%+v err=%#v", failed, failureErr)
	}
	if diagnostic.count() == 0 || !diagnostic.contains(rawProviderErrorText) {
		t.Fatalf("raw provider failure did not stay inside diagnostic boundary")
	}
	failedEvent, err := repositories.Events.GetByID(ctx, failed.EventID)
	if err != nil {
		t.Fatalf("load failed event: %v", err)
	}
	if containsSecret(failedEvent) || containsSecret(failed) {
		t.Fatalf("failed public/durable state leaked sensitive data")
	}

	adapter.setError(errors.New("retryable provider failure"))
	retryable, retryableErr := module.Events().EnqueueWithReceipt(ctx, events.IntakeRequest{
		DefinitionCode: "scheduled", Recipients: []string{"subject-retry"},
		Locale: "en", IdempotencyKey: "retryable-once",
	})
	if retryableErr == nil || retryable.Status != receipts.StatusFailed {
		t.Fatalf("persistable failure did not produce retry state: receipt=%+v err=%v", retryable, retryableErr)
	}
	adapter.setError(nil)
	sendsBeforeRetry := adapter.count()
	retried, retryErr := module.RetryWithReceipt(ctx, events.RetryRequest{
		EventID: retryable.EventID, IdempotencyKey: "retry-operation-once",
	})
	if retryErr != nil || retried.Status != receipts.StatusProcessed ||
		adapter.count() != sendsBeforeRetry+1 {
		t.Fatalf("positive retry did not deliver exactly once: receipt=%+v sends=%d err=%v", retried, adapter.count(), retryErr)
	}
	retriedReplay, retryReplayErr := module.RetryWithReceipt(ctx, events.RetryRequest{
		EventID: retryable.EventID, IdempotencyKey: "retry-operation-once",
	})
	if retryReplayErr != nil || !retriedReplay.Replay || adapter.count() != sendsBeforeRetry+1 {
		t.Fatalf("retry replay redelivered: receipt=%+v sends=%d err=%v", retriedReplay, adapter.count(), retryReplayErr)
	}

	adapter.setError(nil)
	q.setError(errors.New("queue unavailable"))
	scheduled, scheduleErr := module.Events().EnqueueWithReceipt(ctx, events.IntakeRequest{
		DefinitionCode: "scheduled", Recipients: []string{"subject-scheduled"},
		ScheduleAt: time.Now().Add(time.Hour), IdempotencyKey: "scheduled-once",
	})
	var scheduleSafe privacy.SafeError
	if !errors.As(scheduleErr, &scheduleSafe) || scheduleSafe.Code != "publication_enqueue_failed" ||
		scheduled.Status != receipts.StatusFailed {
		t.Fatalf("persist-before-enqueue failure not exposed: receipt=%+v err=%#v", scheduled, scheduleErr)
	}
	q.setError(nil)
	if recoveryErr := module.RecoverPending(ctx, 10); recoveryErr != nil {
		t.Fatalf("recover pending publication: %v", recoveryErr)
	}
	job, ok := q.last()
	if !ok {
		t.Fatalf("recovery did not publish durable work")
	}
	payload, ok := job.Payload.(events.PublicationJobPayload)
	if !ok || containsSecret(job) {
		t.Fatalf("queue payload is unsafe or unstable: %#v", job.Payload)
	}
	processed, err := module.Events().ProcessPublication(ctx, payload)
	if err != nil || processed.Status != receipts.StatusProcessed {
		t.Fatalf("process recovered publication: receipt=%+v err=%v", processed, err)
	}
	duplicate, err := module.Events().ProcessPublication(ctx, payload)
	if err != nil || !duplicate.Replay || adapter.countForSubject(scheduledDestination) != 1 {
		t.Fatalf("duplicate publication redelivered: receipt=%+v err=%v", duplicate, err)
	}

	active := &domain.NotificationEvent{
		DefinitionCode: "scheduled", TenantID: "", Status: domain.EventStatusPending,
		Recipients: domain.StringList{"active-subject"},
	}
	if createErr := repositories.Events.Create(ctx, active); createErr != nil {
		t.Fatalf("create active retention control: %v", createErr)
	}
	hostEvidence := map[uuid.UUID]string{first.EventID: "credential-delivery-recorded"}
	cutoff := time.Now().UTC()
	purgeRequest := retention.Request{
		EventsBefore: cutoff, MessagesBefore: cutoff, AttemptsBefore: cutoff,
		InboxBefore: cutoff, PublicationsBefore: cutoff, RetryOperationsBefore: cutoff,
		BatchSize: 3,
	}
	deleted := 0
	for run := range 20 {
		result, purgeErr := module.Commands().PurgeRetention.Run(ctx, purgeRequest)
		if purgeErr != nil {
			t.Fatalf("retention command run %d: %v", run, purgeErr)
		}
		deleted += result.EventsDeleted + result.MessagesDeleted + result.AttemptsDeleted + result.InboxDeleted +
			result.PublicationsDeleted + result.RetryOperationsDeleted
		if !result.HasMore {
			break
		}
		if run == 19 {
			t.Fatalf("retention did not converge")
		}
	}
	if deleted == 0 || hostEvidence[first.EventID] == "" {
		t.Fatalf("retention did not delete package records or invalidated host evidence: deleted=%d evidence=%v", deleted, hostEvidence)
	}
	if _, getErr := repositories.Events.GetByID(ctx, active.ID); getErr != nil {
		t.Fatalf("retention deleted active work: %v", getErr)
	}
	if _, lookupErr := module.LookupReceipt(ctx, events.ReceiptLookup{
		DefinitionCode: "sensitive", IdempotencyScope: "system", IdempotencyKey: "sensitive-once",
	}); lookupErr == nil {
		t.Fatalf("retained receipt remained after eligible event purge")
	}
	if _, inspectionErr := module.Deliveries().Get(ctx, deliveries.GetQuery{Scope: "system", EventID: first.EventID}); inspectionErr == nil {
		t.Fatalf("inspection returned a purged delivery")
	}
	directResult, err := module.Retention().Purge(ctx, purgeRequest)
	if err != nil || directResult.HasMore {
		t.Fatalf("direct retention did not share converged service behavior: result=%+v err=%v", directResult, err)
	}
}

func seedSecurityDefinitions(t *testing.T, ctx context.Context, module *notifier.Module) {
	t.Helper()
	if _, err := module.Definitions().Upsert(ctx, definitions.UpsertInput{
		Code: "sensitive", Channels: []string{"email:security", "inbox"},
		TemplateIDs: []string{"email:sensitive-email", "inbox:sensitive-inbox"},
		Policy: map[string]any{
			"persistence_mode":        "metadata_only",
			"inbox_persistence_mode":  "state_only",
			"transient_required_keys": []any{"credential_url"},
			"transient_allowed_keys":  []any{"credential_url"},
		},
	}); err != nil {
		t.Fatalf("seed sensitive definition: %v", err)
	}
	for _, channel := range []string{"email", "inbox"} {
		if _, err := module.Templates().Create(ctx, templates.TemplateInput{
			Code: "sensitive-" + channel, Channel: channel, Locale: "en",
			Subject: "Sensitive", Body: "Open {{ credential_url }}",
		}); err != nil {
			t.Fatalf("seed sensitive %s template: %v", channel, err)
		}
	}
	if _, err := module.Definitions().Upsert(ctx, definitions.UpsertInput{
		Code: "scheduled", Channels: []string{"email:security"},
		TemplateIDs: []string{"email:scheduled-email"},
	}); err != nil {
		t.Fatalf("seed scheduled definition: %v", err)
	}
	if _, err := module.Templates().Create(ctx, templates.TemplateInput{
		Code: "scheduled-email", Channel: "email", Locale: "en",
		Subject: "Scheduled", Body: "Safe body",
	}); err != nil {
		t.Fatalf("seed scheduled template: %v", err)
	}
}

func setupMemorySecurityProvider(*testing.T) (storage.Providers, func()) {
	return storage.NewMemoryProviders(), func() {}
}

func setupSQLiteSecurityProvider(t *testing.T) (storage.Providers, func()) {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.DriverName(), "file:security-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	applySecurityMigrations(t, db, "sqlite")
	return storage.NewBunProviders(db), func() {
		if err := db.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	}
}

func setupPostgresSecurityProvider(t *testing.T) (storage.Providers, func()) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NOTIFICATIONS_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("NOTIFICATIONS_POSTGRES_DSN is not configured")
	}
	adminDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	schema := "notifications_security_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, schemaErr := adminDB.ExecContext(context.Background(), `CREATE SCHEMA `+schema); schemaErr != nil {
		t.Fatalf("create postgres schema: %v", schemaErr)
	}
	sqldb, err := sql.Open("postgres", postgresDSNWithSearchPath(dsn, schema))
	if err != nil {
		t.Fatalf("open schema-scoped postgres: %v", err)
	}
	sqldb.SetMaxOpenConns(8)
	db := bun.NewDB(sqldb, pgdialect.New())
	applySecurityMigrations(t, db, "postgres")
	return storage.NewBunProviders(db), func() {
		if err := db.Close(); err != nil {
			t.Errorf("close postgres: %v", err)
		}
		if _, err := adminDB.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop postgres schema: %v", err)
		}
		if err := adminDB.Close(); err != nil {
			t.Errorf("close postgres admin connection: %v", err)
		}
	}
}

func postgresDSNWithSearchPath(dsn, schema string) string {
	parsed, err := url.Parse(dsn)
	if err == nil && parsed.Scheme != "" {
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	return strings.TrimSpace(dsn) + " search_path=" + schema
}

func applySecurityMigrations(t *testing.T, db *bun.DB, dialect string) {
	t.Helper()
	root, err := notifications.GetMigrationsFS()
	if err != nil {
		t.Fatalf("migration fs: %v", err)
	}
	for _, name := range []string{
		"001_notifications_core.up.sql",
		"002_notification_delivery_upgrades.up.sql",
		"003_notification_delivery_integrity.up.sql",
		"004_notification_retention_indexes.up.sql",
		"005_notification_delivery_inspection_indexes.up.sql",
	} {
		body, err := fs.ReadFile(root, dialect+"/"+name)
		if err != nil {
			t.Fatalf("read %s migration %s: %v", dialect, name, err)
		}
		if _, err := db.ExecContext(context.Background(), string(body)); err != nil {
			t.Fatalf("apply %s migration %s: %v", dialect, name, err)
		}
	}
}

func securityTranslator(t *testing.T) i18n.Translator {
	t.Helper()
	catalog := &i18n.TranslationCatalog{
		Locale:   i18n.Locale{Code: "en"},
		Messages: map[string]i18n.Message{},
	}
	translator, err := i18n.NewSimpleTranslator(
		i18n.NewStaticStore(i18n.Translations{"en": catalog}),
		i18n.WithTranslatorDefaultLocale("en"),
	)
	if err != nil {
		t.Fatalf("new translator: %v", err)
	}
	return translator
}

func containsSecret(value any) bool {
	body, err := json.Marshal(value)
	if err != nil {
		body = fmt.Append(nil, value)
	}
	text := string(body)
	return strings.Contains(text, "secret-123") ||
		strings.Contains(text, resolvedDestination) ||
		strings.Contains(text, failureDestination) ||
		strings.Contains(text, scheduledDestination) ||
		strings.Contains(text, rawProviderErrorText)
}

type securityAdapter struct {
	mu       sync.Mutex
	messages []adapters.Message
	err      error
}

func (*securityAdapter) Name() string { return "security" }
func (*securityAdapter) Capabilities() adapters.Capability {
	return adapters.Capability{Name: "security", Channels: []string{"email"}}
}
func (a *securityAdapter) Send(_ context.Context, message adapters.Message) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messages = append(a.messages, message)
	return a.err
}
func (a *securityAdapter) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.err = err
}
func (a *securityAdapter) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.messages)
}
func (a *securityAdapter) contains(value string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Contains(fmt.Sprint(a.messages), value)
}
func (a *securityAdapter) destinationSeen(value string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, message := range a.messages {
		if message.To == value {
			return true
		}
	}
	return false
}
func (a *securityAdapter) countForSubject(value string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	count := 0
	for _, message := range a.messages {
		if message.To == value {
			count++
		}
	}
	return count
}

type securityResolver struct{}

func (*securityResolver) Resolve(_ context.Context, request adapters.RecipientRequest) (adapters.ResolvedRecipient, error) {
	destination := resolvedDestination
	switch request.SubjectID {
	case "subject-failure":
		destination = failureDestination
	case "subject-scheduled":
		destination = scheduledDestination
	}
	return adapters.ResolvedRecipient{Destination: destination}, nil
}

type securityQueue struct {
	mu   sync.Mutex
	jobs []queue.Job
	err  error
}

func (q *securityQueue) Enqueue(_ context.Context, job queue.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, job)
	return nil
}
func (q *securityQueue) setError(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.err = err
}
func (q *securityQueue) last() (queue.Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.jobs) == 0 {
		return queue.Job{}, false
	}
	return q.jobs[len(q.jobs)-1], true
}

type securityBroadcaster struct {
	mu     sync.Mutex
	events []broadcaster.Event
}

func (b *securityBroadcaster) Broadcast(_ context.Context, event broadcaster.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}
func (b *securityBroadcaster) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

type securityActivity struct {
	mu     sync.Mutex
	events []activity.Event
}

func (a *securityActivity) Notify(_ context.Context, event activity.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
}
func (a *securityActivity) containsSecret() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return containsSecret(a.events)
}

type securityLogger struct {
	mu      sync.Mutex
	entries []any
}

func (l *securityLogger) append(message string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, message, args)
}
func (l *securityLogger) Trace(message string, args ...any) { l.append(message, args...) }
func (l *securityLogger) Debug(message string, args ...any) { l.append(message, args...) }
func (l *securityLogger) Info(message string, args ...any)  { l.append(message, args...) }
func (l *securityLogger) Warn(message string, args ...any)  { l.append(message, args...) }
func (l *securityLogger) Error(message string, args ...any) { l.append(message, args...) }
func (l *securityLogger) Fatal(message string, args ...any) { l.append(message, args...) }
func (l *securityLogger) WithContext(context.Context) logger.Logger {
	return l
}
func (l *securityLogger) containsSecret() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return containsSecret(l.entries)
}

type securityLinkBuilder struct {
	mu       sync.Mutex
	requests []links.LinkRequest
}

func (b *securityLinkBuilder) Build(_ context.Context, request links.LinkRequest) (links.ResolvedLinks, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = append(b.requests, request)
	return links.ResolvedLinks{
		ActionURL: sensitiveValue,
		Records:   []links.LinkRecord{{URL: sensitiveValue, Recipient: request.Recipient}},
	}, nil
}
func (b *securityLinkBuilder) contains(value string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Contains(fmt.Sprint(b.requests), value)
}

type securityLinkStore struct {
	mu      sync.Mutex
	records []links.LinkRecord
}

func (s *securityLinkStore) Save(_ context.Context, records []links.LinkRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, records...)
	return nil
}
func (s *securityLinkStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

type securityLinkObserver struct {
	mu          sync.Mutex
	resolutions []links.LinkResolution
}

func (o *securityLinkObserver) OnLinksResolved(_ context.Context, resolution links.LinkResolution) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.resolutions = append(o.resolutions, resolution)
}
func (o *securityLinkObserver) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.resolutions)
}

type securityDiagnostic struct {
	mu     sync.Mutex
	events []privacy.DiagnosticEvent
}

func (d *securityDiagnostic) Report(_ context.Context, event privacy.DiagnosticEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, event)
}
func (d *securityDiagnostic) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.events)
}
func (d *securityDiagnostic) contains(value string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, event := range d.events {
		if event.Cause != nil && strings.Contains(event.Cause.Error(), value) {
			return true
		}
	}
	return false
}

var _ broadcaster.Broadcaster = (*securityBroadcaster)(nil)
var _ logger.Logger = (*securityLogger)(nil)
