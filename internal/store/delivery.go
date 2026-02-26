package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zachbroad/webhook-relay/internal/model"
)

type DeliveryStore struct {
	pool *pgxpool.Pool
}

func (s *DeliveryStore) Create(ctx context.Context, sourceID uuid.UUID, idempotencyKey string, headers, payload json.RawMessage) (*model.Delivery, error) {
	var d model.Delivery
	err := s.pool.QueryRow(ctx,
		`INSERT INTO deliveries (source_id, idempotency_key, headers, payload)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, source_id, idempotency_key, headers, payload, status, received_at, transformed_payload, transformed_headers`,
		sourceID, idempotencyKey, headers, payload,
	).Scan(&d.ID, &d.SourceID, &d.IdempotencyKey, &d.Headers, &d.Payload, &d.Status, &d.ReceivedAt, &d.TransformedPayload, &d.TransformedHeaders)
	if err != nil {
		return nil, fmt.Errorf("create delivery: %w", err)
	}
	return &d, nil
}

func (s *DeliveryStore) GetByID(ctx context.Context, id uuid.UUID) (*model.Delivery, error) {
	var d model.Delivery
	err := s.pool.QueryRow(ctx,
		`SELECT id, source_id, idempotency_key, headers, payload, status, received_at, transformed_payload, transformed_headers
		 FROM deliveries WHERE id = $1`,
		id,
	).Scan(&d.ID, &d.SourceID, &d.IdempotencyKey, &d.Headers, &d.Payload, &d.Status, &d.ReceivedAt, &d.TransformedPayload, &d.TransformedHeaders)
	if err != nil {
		return nil, fmt.Errorf("get delivery: %w", err)
	}
	return &d, nil
}

func (s *DeliveryStore) List(ctx context.Context, sourceSlug *string, limit int) ([]model.Delivery, error) {
	query := `SELECT d.id, d.source_id, d.idempotency_key, d.headers, d.payload, d.status, d.received_at, d.transformed_payload, d.transformed_headers
		 FROM deliveries d`
	args := []any{}
	argIdx := 1

	if sourceSlug != nil {
		query += fmt.Sprintf(` JOIN sources s ON d.source_id = s.id WHERE s.slug = $%d`, argIdx)
		args = append(args, *sourceSlug)
		argIdx++
	}

	query += ` ORDER BY d.received_at DESC`
	query += fmt.Sprintf(` LIMIT $%d`, argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []model.Delivery
	for rows.Next() {
		var d model.Delivery
		if err := rows.Scan(&d.ID, &d.SourceID, &d.IdempotencyKey, &d.Headers, &d.Payload, &d.Status, &d.ReceivedAt, &d.TransformedPayload, &d.TransformedHeaders); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

func (s *DeliveryStore) UpdateStatus(ctx context.Context, id uuid.UUID, status model.DeliveryStatus) error {
	_, err := s.pool.Exec(ctx, `UPDATE deliveries SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("update delivery status: %w", err)
	}
	return nil
}

func (s *DeliveryStore) SetTransformed(ctx context.Context, id uuid.UUID, payload, headers json.RawMessage) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE deliveries SET transformed_payload = $2, transformed_headers = $3 WHERE id = $1`,
		id, payload, headers,
	)
	if err != nil {
		return fmt.Errorf("set transformed: %w", err)
	}
	return nil
}

func (s *DeliveryStore) ListPending(ctx context.Context, limit int) ([]model.Delivery, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, source_id, idempotency_key, headers, payload, status, received_at, transformed_payload, transformed_headers
		 FROM deliveries WHERE status = 'pending' ORDER BY received_at ASC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []model.Delivery
	for rows.Next() {
		var d model.Delivery
		if err := rows.Scan(&d.ID, &d.SourceID, &d.IdempotencyKey, &d.Headers, &d.Payload, &d.Status, &d.ReceivedAt, &d.TransformedPayload, &d.TransformedHeaders); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// Attempt operations

func (s *DeliveryStore) CreateAttempt(ctx context.Context, deliveryID, actionID uuid.UUID, attemptNumber int) (*model.DeliveryAttempt, error) {
	var a model.DeliveryAttempt
	err := s.pool.QueryRow(ctx,
		`INSERT INTO delivery_attempts (delivery_id, action_id, attempt_number)
		 VALUES ($1, $2, $3)
		 RETURNING id, delivery_id, action_id, attempt_number, status, response_status, response_body, error_message, next_retry_at, created_at`,
		deliveryID, actionID, attemptNumber,
	).Scan(&a.ID, &a.DeliveryID, &a.ActionID, &a.AttemptNumber, &a.Status, &a.ResponseStatus, &a.ResponseBody, &a.ErrorMessage, &a.NextRetryAt, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create attempt: %w", err)
	}
	return &a, nil
}

func (s *DeliveryStore) UpdateAttempt(ctx context.Context, id uuid.UUID, status model.AttemptStatus, responseStatus *int, responseBody *string, errorMessage *string, nextRetryAt *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE delivery_attempts SET
			status          = $2,
			response_status = $3,
			response_body   = $4,
			error_message   = $5,
			next_retry_at   = $6
		 WHERE id = $1`,
		id, status, responseStatus, responseBody, errorMessage, nextRetryAt,
	)
	if err != nil {
		return fmt.Errorf("update attempt: %w", err)
	}
	return nil
}

func (s *DeliveryStore) ListRetryableAttempts(ctx context.Context, limit int) ([]model.DeliveryAttempt, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, delivery_id, action_id, attempt_number, status, response_status, response_body, error_message, next_retry_at, created_at
		 FROM delivery_attempts
		 WHERE status = 'failed' AND next_retry_at IS NOT NULL AND next_retry_at <= now()
		 ORDER BY next_retry_at ASC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list retryable attempts: %w", err)
	}
	defer rows.Close()

	var attempts []model.DeliveryAttempt
	for rows.Next() {
		var a model.DeliveryAttempt
		if err := rows.Scan(&a.ID, &a.DeliveryID, &a.ActionID, &a.AttemptNumber, &a.Status, &a.ResponseStatus, &a.ResponseBody, &a.ErrorMessage, &a.NextRetryAt, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attempt: %w", err)
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

func (s *DeliveryStore) ListAttemptsByDelivery(ctx context.Context, deliveryID uuid.UUID) ([]model.DeliveryAttempt, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, delivery_id, action_id, attempt_number, status, response_status, response_body, error_message, next_retry_at, created_at
		 FROM delivery_attempts
		 WHERE delivery_id = $1
		 ORDER BY created_at ASC`,
		deliveryID,
	)
	if err != nil {
		return nil, fmt.Errorf("list attempts by delivery: %w", err)
	}
	defer rows.Close()

	var attempts []model.DeliveryAttempt
	for rows.Next() {
		var a model.DeliveryAttempt
		if err := rows.Scan(&a.ID, &a.DeliveryID, &a.ActionID, &a.AttemptNumber, &a.Status, &a.ResponseStatus, &a.ResponseBody, &a.ErrorMessage, &a.NextRetryAt, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attempt: %w", err)
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

func (s *DeliveryStore) GetMaxAttemptNumber(ctx context.Context, deliveryID, actionID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(attempt_number), 0) FROM delivery_attempts WHERE delivery_id = $1 AND action_id = $2`,
		deliveryID, actionID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("get max attempt number: %w", err)
	}
	return n, nil
}
