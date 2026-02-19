package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zachbroad/webhook-relay/internal/model"
)

type SourceStore struct {
	pool *pgxpool.Pool
}

func (s *SourceStore) GetBySlug(ctx context.Context, slug string) (*model.Source, error) {
	var src model.Source
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at, updated_at FROM sources WHERE slug = $1`,
		slug,
	).Scan(&src.ID, &src.Name, &src.Slug, &src.CreatedAt, &src.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get source by slug: %w", err)
	}
	return &src, nil
}

func (s *SourceStore) GetByID(ctx context.Context, id uuid.UUID) (*model.Source, error) {
	var src model.Source
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at, updated_at FROM sources WHERE id = $1`,
		id,
	).Scan(&src.ID, &src.Name, &src.Slug, &src.CreatedAt, &src.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get source by id: %w", err)
	}
	return &src, nil
}
