package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zachbroad/webhook-relay/internal/model"
)

type SubscriptionStore struct {
	pool *pgxpool.Pool
}

func (s *SubscriptionStore) Create(ctx context.Context, sourceID uuid.UUID, targetURL string, signingSecret *string) (*model.Subscription, error) {
	var sub model.Subscription
	err := s.pool.QueryRow(ctx,
		`INSERT INTO subscriptions (source_id, target_url, signing_secret)
		 VALUES ($1, $2, $3)
		 RETURNING id, source_id, target_url, signing_secret, is_active, created_at, updated_at`,
		sourceID, targetURL, signingSecret,
	).Scan(&sub.ID, &sub.SourceID, &sub.TargetURL, &sub.SigningSecret, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}
	return &sub, nil
}

func (s *SubscriptionStore) List(ctx context.Context, sourceID uuid.UUID) ([]model.Subscription, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, source_id, target_url, signing_secret, is_active, created_at, updated_at
		 FROM subscriptions WHERE source_id = $1 ORDER BY created_at DESC`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []model.Subscription
	for rows.Next() {
		var sub model.Subscription
		if err := rows.Scan(&sub.ID, &sub.SourceID, &sub.TargetURL, &sub.SigningSecret, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *SubscriptionStore) GetByID(ctx context.Context, id uuid.UUID) (*model.Subscription, error) {
	var sub model.Subscription
	err := s.pool.QueryRow(ctx,
		`SELECT id, source_id, target_url, signing_secret, is_active, created_at, updated_at
		 FROM subscriptions WHERE id = $1`,
		id,
	).Scan(&sub.ID, &sub.SourceID, &sub.TargetURL, &sub.SigningSecret, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return &sub, nil
}

func (s *SubscriptionStore) Update(ctx context.Context, id uuid.UUID, targetURL *string, signingSecret *string, isActive *bool) (*model.Subscription, error) {
	var sub model.Subscription
	err := s.pool.QueryRow(ctx,
		`UPDATE subscriptions SET
			target_url     = COALESCE($2, target_url),
			signing_secret = COALESCE($3, signing_secret),
			is_active      = COALESCE($4, is_active),
			updated_at     = $5
		 WHERE id = $1
		 RETURNING id, source_id, target_url, signing_secret, is_active, created_at, updated_at`,
		id, targetURL, signingSecret, isActive, time.Now(),
	).Scan(&sub.ID, &sub.SourceID, &sub.TargetURL, &sub.SigningSecret, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update subscription: %w", err)
	}
	return &sub, nil
}

func (s *SubscriptionStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}

func (s *SubscriptionStore) ListActiveBySource(ctx context.Context, sourceID uuid.UUID) ([]model.Subscription, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, source_id, target_url, signing_secret, is_active, created_at, updated_at
		 FROM subscriptions WHERE source_id = $1 AND is_active = true`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []model.Subscription
	for rows.Next() {
		var sub model.Subscription
		if err := rows.Scan(&sub.ID, &sub.SourceID, &sub.TargetURL, &sub.SigningSecret, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}
