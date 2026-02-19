package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Source struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Subscription struct {
	ID            uuid.UUID `json:"id"`
	SourceID      uuid.UUID `json:"source_id"`
	TargetURL     string    `json:"target_url"`
	SigningSecret *string   `json:"signing_secret,omitempty"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type DeliveryStatus string

const (
	DeliveryPending    DeliveryStatus = "pending"
	DeliveryProcessing DeliveryStatus = "processing"
	DeliveryCompleted  DeliveryStatus = "completed"
	DeliveryFailed     DeliveryStatus = "failed"
)

type Delivery struct {
	ID             uuid.UUID       `json:"id"`
	SourceID       uuid.UUID       `json:"source_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Headers        json.RawMessage `json:"headers"`
	Payload        json.RawMessage `json:"payload"`
	Status         DeliveryStatus  `json:"status"`
	ReceivedAt     time.Time       `json:"received_at"`
}

type AttemptStatus string

const (
	AttemptPending AttemptStatus = "pending"
	AttemptSuccess AttemptStatus = "success"
	AttemptFailed  AttemptStatus = "failed"
)

type DeliveryAttempt struct {
	ID             uuid.UUID     `json:"id"`
	DeliveryID     uuid.UUID     `json:"delivery_id"`
	SubscriptionID uuid.UUID     `json:"subscription_id"`
	AttemptNumber  int           `json:"attempt_number"`
	Status         AttemptStatus `json:"status"`
	ResponseStatus *int          `json:"response_status,omitempty"`
	ResponseBody   *string       `json:"response_body,omitempty"`
	ErrorMessage   *string       `json:"error_message,omitempty"`
	NextRetryAt    *time.Time    `json:"next_retry_at,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}
