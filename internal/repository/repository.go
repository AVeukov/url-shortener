package repository

import (
	"context"
	"errors"

	"github.com/AVeukov/url-shortener/internal/model"
)

var (
	ErrNotFound = errors.New("link not found")
	ErrConflict = errors.New("short code already exists")
)

type LinkRepository interface {
	Create(ctx context.Context, link model.Link) error
	Get(ctx context.Context, code string) (model.Link, error)
	IncrementClicks(ctx context.Context, code string) error
}
