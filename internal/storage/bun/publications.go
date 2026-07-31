package bunrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var errDigestMembershipRace = errors.New("digest membership changed concurrently")

type PublicationRepository struct {
	crudRepository[domain.NotificationPublication]
}

func NewPublicationRepository(db *bun.DB) *PublicationRepository {
	return &PublicationRepository{
		crudRepository: newEntityCRUD(db, "id",
			func(publication *domain.NotificationPublication) *domain.RecordMeta { return &publication.RecordMeta }, nil),
	}
}

func (r *PublicationRepository) Create(ctx context.Context, value *domain.NotificationPublication) error {
	if value.Status == "" {
		value.Status = domain.PublicationStatusPending
	}
	return r.base.create(ctx, value)
}

func (r *PublicationRepository) ListPending(ctx context.Context, limit int) ([]domain.NotificationPublication, error) {
	now := time.Now().UTC()
	query := r.base.db.NewSelect().
		Model((*domain.NotificationPublication)(nil)).
		Where("(status = ? OR (status = ? AND claim_until <= ?))",
			domain.PublicationStatusPending, domain.PublicationStatusProcessing, now).
		Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var items []domain.NotificationPublication
	if err := query.Scan(ctx, &items); err != nil {
		return nil, mapError(err)
	}
	return items, nil
}

func (r *PublicationRepository) FindOpenDigest(ctx context.Context, digestKey string) (*domain.NotificationPublication, error) {
	value := &domain.NotificationPublication{}
	err := r.base.db.NewSelect().
		Model(value).
		Where("digest_key = ?", digestKey).
		Where("status IN (?, ?)", domain.PublicationStatusPending, domain.PublicationStatusPublished).
		Order("created_at ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return value, nil
}

func (r *PublicationRepository) CreateOrGetOpenDigest(ctx context.Context, value *domain.NotificationPublication) (*domain.NotificationPublication, bool, error) {
	if value.Status == "" {
		value.Status = domain.PublicationStatusPending
	}
	var createErr error
	for range 3 {
		createErr = r.base.create(ctx, value)
		if createErr == nil {
			return value, true, nil
		}
		stored, lookupErr := r.FindOpenDigest(ctx, value.DigestKey)
		if lookupErr == nil {
			return stored, false, nil
		}
	}
	return nil, false, createErr
}

// CreateOrAttachOpenDigest keeps the publication lock and event attachment in
// one transaction; retaining the sequence in one function makes the ordering
// against Claim auditable.
//
//nolint:gocyclo,nestif,funlen // Splitting the transaction would obscure its claim-lock ordering.
func (r *PublicationRepository) CreateOrAttachOpenDigest(
	ctx context.Context,
	candidate *domain.NotificationPublication,
	event *domain.NotificationEvent,
) (*domain.NotificationPublication, bool, error) {
	if candidate.Status == "" {
		candidate.Status = domain.PublicationStatusPending
	}
	candidate.EnsureID()
	now := time.Now().UTC()
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = now
	}
	candidate.UpdatedAt = now

	for range 5 {
		var publication domain.NotificationPublication
		created := false
		err := r.base.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
			err := tx.NewSelect().
				Model(&publication).
				Where("digest_key = ?", candidate.DigestKey).
				Where("status IN (?, ?)", domain.PublicationStatusPending, domain.PublicationStatusPublished).
				Order("created_at ASC").
				Limit(1).
				Scan(ctx)
			if err != nil && !errors.Is(mapError(err), store.ErrNotFound) {
				return mapError(err)
			}
			if err != nil {
				publication = *candidate
				result, insertErr := tx.NewInsert().
					Model(&publication).
					On("CONFLICT DO NOTHING").
					Exec(ctx)
				if insertErr != nil {
					return mapError(insertErr)
				}
				count, countErr := result.RowsAffected()
				if countErr != nil {
					return countErr
				}
				if count != 1 {
					return errDigestMembershipRace
				}
				created = true
			} else {
				// Claim uses a conditional update on this same row. This no-op
				// update serializes membership attachment with that transition.
				result, updateErr := tx.NewUpdate().
					Model((*domain.NotificationPublication)(nil)).
					Set("updated_at = updated_at").
					Where("id = ?", publication.ID).
					Where("status IN (?, ?)", domain.PublicationStatusPending, domain.PublicationStatusPublished).
					Exec(ctx)
				if updateErr != nil {
					return mapError(updateErr)
				}
				count, countErr := result.RowsAffected()
				if countErr != nil {
					return countErr
				}
				if count != 1 {
					return errDigestMembershipRace
				}
			}

			event.UpdatedAt = time.Now().UTC()
			result, updateErr := tx.NewUpdate().
				Model((*domain.NotificationEvent)(nil)).
				Set("publication_id = ?", publication.ID).
				Set("digest_key = ?", publication.DigestKey).
				Set("status = ?", domain.EventStatusScheduled).
				Set("updated_at = ?", event.UpdatedAt).
				Where("id = ?", event.ID).
				Exec(ctx)
			if updateErr != nil {
				return mapError(updateErr)
			}
			count, countErr := result.RowsAffected()
			if countErr != nil {
				return countErr
			}
			if count != 1 {
				return store.ErrNotFound
			}
			return nil
		})
		if errors.Is(err, errDigestMembershipRace) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		event.PublicationID = publication.ID
		event.DigestKey = publication.DigestKey
		event.Status = domain.EventStatusScheduled
		return &publication, created, nil
	}
	return nil, false, errDigestMembershipRace
}

func (r *PublicationRepository) Claim(ctx context.Context, id uuid.UUID, until time.Time) (bool, error) {
	now := time.Now().UTC()
	result, err := r.base.db.NewUpdate().
		Model((*domain.NotificationPublication)(nil)).
		Set("status = ?", domain.PublicationStatusProcessing).
		Set("claim_until = ?", until).
		Set("attempts = attempts + 1").
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("(status IN (?, ?) OR (status = ? AND (claim_until <= ? OR claim_until IS NULL)))",
			domain.PublicationStatusPending,
			domain.PublicationStatusPublished,
			domain.PublicationStatusProcessing,
			now).
		Exec(ctx)
	if err != nil {
		return false, mapError(err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
