package store

import "github.com/jackc/pgx/v5/pgxpool"

type Store struct {
	Sources       *SourceStore
	Subscriptions *SubscriptionStore
	Deliveries    *DeliveryStore
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		Sources:       &SourceStore{pool: pool},
		Subscriptions: &SubscriptionStore{pool: pool},
		Deliveries:    &DeliveryStore{pool: pool},
	}
}
