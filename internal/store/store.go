package store

import "github.com/jackc/pgx/v5/pgxpool"

type Store struct {
	Sources    *SourceStore
	Actions    *ActionStore
	Deliveries *DeliveryStore
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		Sources:    &SourceStore{pool: pool},
		Actions:    &ActionStore{pool: pool},
		Deliveries: &DeliveryStore{pool: pool},
	}
}
