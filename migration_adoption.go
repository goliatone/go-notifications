package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	persistence "github.com/goliatone/go-persistence-bun"
	"github.com/uptrace/bun"
)

// ErrUnsafeMigrationGraphAdoption means an existing source identity would be
// changed or reordered instead of preserving it during additive adoption.
var ErrUnsafeMigrationGraphAdoption = errors.New("unsafe ordered migration graph adoption")

type orderedSourceIdentityRecord struct {
	bun.BaseModel `bun:"table:bun_ordered_migration_sources"`

	SourceKey        string                      `bun:"source_key,pk"`
	SourceName       string                      `bun:"source_name,notnull"`
	SourceOrder      int                         `bun:"source_order,notnull"`
	Dependencies     persistence.JSONStringSlice `bun:"dependencies,type:json"`
	ResolvedPosition int                         `bun:"resolved_position,notnull"`
	IdentityMode     string                      `bun:"identity_mode,notnull"`
	GraphFingerprint string                      `bun:"graph_fingerprint,notnull"`
	CreatedAt        time.Time                   `bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt        time.Time                   `bun:"updated_at,nullzero,notnull,default:current_timestamp"`
}

// AdoptAdditiveOrderedMigrationGraph explicitly persists an additive graph
// extension before migration. Existing source identity, order, dependencies,
// and position must remain unchanged; every new source must follow all
// persisted sources. Bun migration journal rows are never modified.
func AdoptAdditiveOrderedMigrationGraph(ctx context.Context, db *bun.DB, manager *persistence.Migrations) error {
	if db == nil || manager == nil {
		return fmt.Errorf("%w: database and migration manager are required", ErrUnsafeMigrationGraphAdoption)
	}
	plan, err := manager.Plan(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: resolve proposed graph: %w", ErrUnsafeMigrationGraphAdoption, err)
	}
	expected, fingerprint, err := orderedIdentityRecords(plan)
	if err != nil {
		return err
	}
	expectedByKey := make(map[string]orderedSourceIdentityRecord, len(expected))
	for _, row := range expected {
		expectedByKey[row.SourceKey] = row
	}
	now := time.Now().UTC()
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var persisted []orderedSourceIdentityRecord
		if scanErr := tx.NewSelect().Model(&persisted).Scan(ctx); scanErr != nil {
			return fmt.Errorf("%w: load persisted graph: %w", ErrUnsafeMigrationGraphAdoption, scanErr)
		}
		if len(persisted) == 0 {
			return fmt.Errorf("%w: no persisted source-stable graph to extend", ErrUnsafeMigrationGraphAdoption)
		}
		maxOrder, maxPosition, persistedKeys, validationErr := validatePersistedSources(persisted, expectedByKey)
		if validationErr != nil {
			return validationErr
		}
		if validationErr := validateAdditiveSources(expected, persistedKeys, maxOrder, maxPosition); validationErr != nil {
			return validationErr
		}
		for i := range expected {
			expected[i].GraphFingerprint = fingerprint
			expected[i].UpdatedAt = now
			if _, err := tx.NewInsert().Model(&expected[i]).
				On("CONFLICT (source_key) DO UPDATE").
				Set("graph_fingerprint = EXCLUDED.graph_fingerprint").
				Set("updated_at = EXCLUDED.updated_at").
				Exec(ctx); err != nil {
				return fmt.Errorf("%w: persist source %q: %w", ErrUnsafeMigrationGraphAdoption, expected[i].SourceKey, err)
			}
		}
		return nil
	})
}

func validatePersistedSources(
	persisted []orderedSourceIdentityRecord,
	expectedByKey map[string]orderedSourceIdentityRecord,
) (int, int, map[string]struct{}, error) {
	maxOrder, maxPosition := 0, 0
	persistedKeys := make(map[string]struct{}, len(persisted))
	for _, observed := range persisted {
		candidate, ok := expectedByKey[observed.SourceKey]
		if !ok || !sameOrderedSourceIdentity(observed, candidate) {
			return 0, 0, nil, fmt.Errorf("%w: persisted source %q would change identity", ErrUnsafeMigrationGraphAdoption, observed.SourceKey)
		}
		persistedKeys[observed.SourceKey] = struct{}{}
		maxOrder = max(maxOrder, observed.SourceOrder)
		maxPosition = max(maxPosition, observed.ResolvedPosition)
	}
	return maxOrder, maxPosition, persistedKeys, nil
}

func validateAdditiveSources(
	expected []orderedSourceIdentityRecord,
	persistedKeys map[string]struct{},
	maxOrder int,
	maxPosition int,
) error {
	added := 0
	for _, candidate := range expected {
		if _, exists := persistedKeys[candidate.SourceKey]; exists {
			continue
		}
		added++
		if candidate.SourceOrder <= maxOrder || candidate.ResolvedPosition <= maxPosition {
			return fmt.Errorf("%w: new source %q must follow the persisted graph", ErrUnsafeMigrationGraphAdoption, candidate.SourceKey)
		}
	}
	if added == 0 {
		return fmt.Errorf("%w: proposed graph does not add a source", ErrUnsafeMigrationGraphAdoption)
	}
	return nil
}

func orderedIdentityRecords(plan *persistence.MigrationPlan) ([]orderedSourceIdentityRecord, string, error) {
	if plan == nil {
		return nil, "", fmt.Errorf("%w: migration plan is required", ErrUnsafeMigrationGraphAdoption)
	}
	byKey := make(map[string]orderedSourceIdentityRecord)
	for _, entry := range plan.Entries {
		if entry.SourceKind != persistence.MigrationSourceKindOrdered ||
			entry.IdentityMode != persistence.OrderedMigrationIdentitySourceStable {
			continue
		}
		row := orderedSourceIdentityRecord{
			SourceKey: entry.SourceKey, SourceName: entry.SourceName, SourceOrder: entry.SourceOrder,
			Dependencies: slices.Clone(entry.SourceDependsOn), ResolvedPosition: entry.ResolvedPosition,
			IdentityMode: entry.IdentityMode.String(),
		}
		if existing, ok := byKey[row.SourceKey]; ok && !sameOrderedSourceIdentity(existing, row) {
			return nil, "", fmt.Errorf("%w: inconsistent plan metadata for %q", ErrUnsafeMigrationGraphAdoption, row.SourceKey)
		}
		byKey[row.SourceKey] = row
	}
	rows := make([]orderedSourceIdentityRecord, 0, len(byKey))
	for _, row := range byKey {
		sort.Strings(row.Dependencies)
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SourceKey < rows[j].SourceKey })
	if len(rows) == 0 {
		return nil, "", fmt.Errorf("%w: proposed graph has no source-stable entries", ErrUnsafeMigrationGraphAdoption)
	}
	type fingerprintSource struct {
		SourceKey        string   `json:"source_key"`
		SourceName       string   `json:"source_name"`
		SourceOrder      int      `json:"source_order"`
		Dependencies     []string `json:"dependencies"`
		ResolvedPosition int      `json:"resolved_position"`
		IdentityMode     string   `json:"identity_mode"`
	}
	values := make([]fingerprintSource, len(rows))
	for i, row := range rows {
		values[i] = fingerprintSource{
			SourceKey: row.SourceKey, SourceName: row.SourceName, SourceOrder: row.SourceOrder,
			Dependencies: []string(row.Dependencies), ResolvedPosition: row.ResolvedPosition,
			IdentityMode: row.IdentityMode,
		}
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return nil, "", fmt.Errorf("%w: fingerprint proposed graph: %w", ErrUnsafeMigrationGraphAdoption, err)
	}
	sum := sha256.Sum256(payload)
	return rows, hex.EncodeToString(sum[:]), nil
}

func sameOrderedSourceIdentity(left, right orderedSourceIdentityRecord) bool {
	leftDependencies := slices.Clone(left.Dependencies)
	rightDependencies := slices.Clone(right.Dependencies)
	sort.Strings(leftDependencies)
	sort.Strings(rightDependencies)
	return left.SourceKey == right.SourceKey && left.SourceName == right.SourceName &&
		left.SourceOrder == right.SourceOrder && slices.Equal(leftDependencies, rightDependencies) &&
		left.ResolvedPosition == right.ResolvedPosition && left.IdentityMode == right.IdentityMode
}
