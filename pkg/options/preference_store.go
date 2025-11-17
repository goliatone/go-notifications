package options

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	opts "github.com/goliatone/go-options"
)

// PreferenceScopeRef describes how a stored preference maps to a scope layer.
type PreferenceScopeRef struct {
	Scope          opts.Scope
	SubjectType    string
	SubjectID      string
	DefinitionCode string
	Channel        string
}

// PreferenceSnapshotInput captures the mutable fields persisted for a scope.
type PreferenceSnapshotInput struct {
	Scope      PreferenceScopeRef
	Enabled    *bool
	Locale     *string
	QuietHours domain.JSONMap
	Rules      domain.JSONMap
}

// PreferenceSnapshotStore adapts the preference repository to scope snapshots.
type PreferenceSnapshotStore struct {
	Repository store.NotificationPreferenceRepository
}

var (
	errPreferenceRepositoryRequired = errors.New("options: preference repository is required")
)

// Load pulls domain preferences for the supplied scope references and converts
// them into scope snapshots that can be fed into the resolver.
func (s PreferenceSnapshotStore) Load(ctx context.Context, refs []PreferenceScopeRef) ([]Snapshot, error) {
	if s.Repository == nil {
		return nil, errPreferenceRepositoryRequired
	}

	snapshots := make([]Snapshot, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.SubjectType) == "" || strings.TrimSpace(ref.SubjectID) == "" {
			return nil, fmt.Errorf("options: scope %s missing subject identifiers", ref.Scope.Name)
		}
		if ref.Scope.Name == "" {
			return nil, fmt.Errorf("options: scope name required for %s/%s", ref.SubjectType, ref.SubjectID)
		}
		pref, err := s.Repository.GetBySubject(ctx, ref.SubjectType, ref.SubjectID, ref.DefinitionCode, ref.Channel)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, err
		}
		snapshots = append(snapshots, snapshotFromPreference(ref.Scope, pref))
	}
	return snapshots, nil
}

// Save upserts a preference snapshot for the provided scope reference.
func (s PreferenceSnapshotStore) Save(ctx context.Context, input PreferenceSnapshotInput) (*domain.NotificationPreference, error) {
	if s.Repository == nil {
		return nil, errPreferenceRepositoryRequired
	}
	ref := input.Scope
	if strings.TrimSpace(ref.SubjectType) == "" || strings.TrimSpace(ref.SubjectID) == "" {
		return nil, fmt.Errorf("options: subject identifiers are required")
	}
	if strings.TrimSpace(ref.DefinitionCode) == "" {
		return nil, fmt.Errorf("options: definition code is required")
	}
	if strings.TrimSpace(ref.Channel) == "" {
		return nil, fmt.Errorf("options: channel is required")
	}

	pref, err := s.Repository.GetBySubject(ctx, ref.SubjectType, ref.SubjectID, ref.DefinitionCode, ref.Channel)
	switch {
	case err == nil:
		applySnapshot(pref, input)
		if err := s.Repository.Update(ctx, pref); err != nil {
			return nil, err
		}
		return pref, nil
	case errors.Is(err, store.ErrNotFound):
		record := &domain.NotificationPreference{
			SubjectType:    ref.SubjectType,
			SubjectID:      ref.SubjectID,
			DefinitionCode: ref.DefinitionCode,
			Channel:        ref.Channel,
		}
		applySnapshot(record, input)
		if record.Locale == "" && input.Locale != nil {
			record.Locale = strings.TrimSpace(*input.Locale)
		}
		if input.Enabled == nil {
			record.Enabled = true
		}
		if err := s.Repository.Create(ctx, record); err != nil {
			return nil, err
		}
		return record, nil
	default:
		return nil, err
	}
}

func applySnapshot(pref *domain.NotificationPreference, input PreferenceSnapshotInput) {
	if pref == nil {
		return
	}
	if input.Enabled != nil {
		pref.Enabled = *input.Enabled
	}
	if input.Locale != nil {
		pref.Locale = strings.TrimSpace(*input.Locale)
	}
	if input.QuietHours != nil {
		pref.QuietHours = copyJSONMap(input.QuietHours)
	}
	if input.Rules != nil {
		pref.AdditionalRules = copyJSONMap(input.Rules)
	}
}

func snapshotFromPreference(scope opts.Scope, pref *domain.NotificationPreference) Snapshot {
	payload := map[string]any{
		"enabled": pref.Enabled,
	}
	if pref.DefinitionCode != "" {
		payload["definition"] = pref.DefinitionCode
	}
	if pref.Channel != "" {
		payload["channel"] = pref.Channel
	}
	if pref.Locale != "" {
		payload["locale"] = pref.Locale
	}
	if len(pref.QuietHours) > 0 {
		payload["quiet_hours"] = copyJSONMap(pref.QuietHours)
	}
	if len(pref.AdditionalRules) > 0 {
		payload["rules"] = copyJSONMap(pref.AdditionalRules)
	}
	return Snapshot{
		Scope:      scope,
		Data:       payload,
		SnapshotID: pref.ID.String(),
	}
}

func copyJSONMap(src domain.JSONMap) domain.JSONMap {
	if len(src) == 0 {
		return nil
	}
	cloned := cloneMap(map[string]any(src))
	if cloned == nil {
		return nil
	}
	return domain.JSONMap(cloned)
}
