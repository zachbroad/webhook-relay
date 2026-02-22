package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zachbroad/webhook-relay/internal/model"
)

type SourceStore struct {
	pool *pgxpool.Pool
}

func (s *SourceStore) GetBySlug(ctx context.Context, slug string) (*model.Source, error) {
	var src model.Source
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, mode, script_body, created_at, updated_at FROM sources WHERE slug = $1`,
		slug,
	).Scan(&src.ID, &src.Name, &src.Slug, &src.Mode, &src.ScriptBody, &src.CreatedAt, &src.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get source by slug: %w", err)
	}
	return &src, nil
}

func (s *SourceStore) GetByID(ctx context.Context, id uuid.UUID) (*model.Source, error) {
	var src model.Source
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, mode, script_body, created_at, updated_at FROM sources WHERE id = $1`,
		id,
	).Scan(&src.ID, &src.Name, &src.Slug, &src.Mode, &src.ScriptBody, &src.CreatedAt, &src.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get source by id: %w", err)
	}
	return &src, nil
}

func (s *SourceStore) List(ctx context.Context) ([]model.Source, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, mode, script_body, created_at, updated_at FROM sources ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	defer rows.Close()

	var sources []model.Source
	for rows.Next() {
		var src model.Source
		if err := rows.Scan(&src.ID, &src.Name, &src.Slug, &src.Mode, &src.ScriptBody, &src.CreatedAt, &src.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		sources = append(sources, src)
	}
	return sources, rows.Err()
}

func (s *SourceStore) Create(ctx context.Context, name, slug, mode string, scriptBody *string) (*model.Source, error) {
	var src model.Source
	err := s.pool.QueryRow(ctx,
		`INSERT INTO sources (name, slug, mode, script_body) VALUES ($1, $2, $3, $4)
		 RETURNING id, name, slug, mode, script_body, created_at, updated_at`,
		name, slug, mode, scriptBody,
	).Scan(&src.ID, &src.Name, &src.Slug, &src.Mode, &src.ScriptBody, &src.CreatedAt, &src.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create source: %w", err)
	}
	return &src, nil
}

func (s *SourceStore) Update(ctx context.Context, slug string, name *string, mode *string, scriptBody *string, clearScript bool) (*model.Source, error) {
	var src model.Source
	// If clearScript is true, we explicitly set script_body to NULL.
	// Otherwise we use COALESCE to keep existing value when scriptBody is nil.
	var scriptArg any
	if clearScript {
		scriptArg = nil
	} else if scriptBody != nil {
		scriptArg = *scriptBody
	}

	var err error
	if clearScript {
		err = s.pool.QueryRow(ctx,
			`UPDATE sources SET
				name        = COALESCE($2, name),
				mode        = COALESCE($3, mode),
				script_body = NULL,
				updated_at  = $4
			 WHERE slug = $1
			 RETURNING id, name, slug, mode, script_body, created_at, updated_at`,
			slug, name, mode, time.Now(),
		).Scan(&src.ID, &src.Name, &src.Slug, &src.Mode, &src.ScriptBody, &src.CreatedAt, &src.UpdatedAt)
	} else {
		err = s.pool.QueryRow(ctx,
			`UPDATE sources SET
				name        = COALESCE($2, name),
				mode        = COALESCE($3, mode),
				script_body = COALESCE($4, script_body),
				updated_at  = $5
			 WHERE slug = $1
			 RETURNING id, name, slug, mode, script_body, created_at, updated_at`,
			slug, name, mode, scriptArg, time.Now(),
		).Scan(&src.ID, &src.Name, &src.Slug, &src.Mode, &src.ScriptBody, &src.CreatedAt, &src.UpdatedAt)
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("source not found")
		}
		return nil, fmt.Errorf("update source: %w", err)
	}
	return &src, nil
}

func (s *SourceStore) Delete(ctx context.Context, slug string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM sources WHERE slug = $1`, slug)
	if err != nil {
		return fmt.Errorf("delete source: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("source not found")
	}
	return nil
}
