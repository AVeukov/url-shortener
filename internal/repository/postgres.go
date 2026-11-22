package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/AVeukov/url-shortener/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close() { p.pool.Close() }

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

func (p *Postgres) Create(ctx context.Context, link model.Link) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO short_links (code, original_url, created_at, expires_at)
		VALUES ($1, $2, $3, $4)`,
		link.Code, link.OriginalURL, link.CreatedAt, link.ExpiresAt,
	)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return fmt.Errorf("insert link: %w", err)
}

func (p *Postgres) Get(ctx context.Context, code string) (model.Link, error) {
	var link model.Link
	err := p.pool.QueryRow(ctx, `
		SELECT code, original_url, created_at, expires_at, click_count
		FROM short_links
		WHERE code = $1 AND (expires_at IS NULL OR expires_at > NOW())`, code,
	).Scan(&link.Code, &link.OriginalURL, &link.CreatedAt, &link.ExpiresAt, &link.ClickCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Link{}, ErrNotFound
	}
	if err != nil {
		return model.Link{}, fmt.Errorf("select link: %w", err)
	}
	return link, nil
}

func (p *Postgres) IncrementClicks(ctx context.Context, code string) error {
	result, err := p.pool.Exec(ctx, `UPDATE short_links SET click_count = click_count + 1 WHERE code = $1`, code)
	if err != nil {
		return fmt.Errorf("increment clicks: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
