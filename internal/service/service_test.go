package service

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AVeukov/url-shortener/internal/cache"
	"github.com/AVeukov/url-shortener/internal/metrics"
	"github.com/AVeukov/url-shortener/internal/model"
	"github.com/AVeukov/url-shortener/internal/repository"
)

type memoryRepository struct{ links map[string]model.Link }

func (m *memoryRepository) Create(_ context.Context, link model.Link) error {
	if _, exists := m.links[link.Code]; exists {
		return repository.ErrConflict
	}
	m.links[link.Code] = link
	return nil
}
func (m *memoryRepository) Get(_ context.Context, code string) (model.Link, error) {
	link, ok := m.links[code]
	if !ok {
		return model.Link{}, repository.ErrNotFound
	}
	return link, nil
}
func (m *memoryRepository) IncrementClicks(_ context.Context, code string) error {
	link, ok := m.links[code]
	if !ok {
		return repository.ErrNotFound
	}
	link.ClickCount++
	m.links[code] = link
	return nil
}

type memoryCache struct{ links map[string]model.Link }

func (m *memoryCache) Get(_ context.Context, code string) (model.Link, error) {
	link, ok := m.links[code]
	if !ok {
		return model.Link{}, cache.ErrMiss
	}
	return link, nil
}
func (m *memoryCache) Set(_ context.Context, link model.Link) error {
	m.links[link.Code] = link
	return nil
}

func newTestService() (*Service, *memoryRepository, *memoryCache) {
	repo := &memoryRepository{links: make(map[string]model.Link)}
	linkCache := &memoryCache{links: make(map[string]model.Link)}
	svc := New(repo, linkCache, metrics.New())
	svc.now = func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) }
	return svc, repo, linkCache
}

func TestCreateWithCustomAlias(t *testing.T) {
	svc, repo, _ := newTestService()
	link, err := svc.Create(context.Background(), "https://example.com/path", "example-link", time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if link.Code != "example-link" || repo.links[link.Code].OriginalURL != "https://example.com/path" {
		t.Fatalf("unexpected link: %+v", link)
	}
	if link.ExpiresAt == nil || !link.ExpiresAt.Equal(link.CreatedAt.Add(time.Hour)) {
		t.Fatalf("unexpected expiration: %v", link.ExpiresAt)
	}
}

func TestCreateRejectsInvalidURL(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Create(context.Background(), "javascript:alert(1)", "", 0)
	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("Create() error = %v, want ErrInvalidURL", err)
	}
}

func TestResolveUsesCache(t *testing.T) {
	svc, _, linkCache := newTestService()
	linkCache.links["cached"] = model.Link{Code: "cached", OriginalURL: "https://example.com"}
	link, err := svc.Resolve(context.Background(), "cached")
	if err != nil || link.OriginalURL != "https://example.com" {
		t.Fatalf("Resolve() = %+v, %v", link, err)
	}
}

func TestGeneratedCode(t *testing.T) {
	svc, _, _ := newTestService()
	svc.random = bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7})
	link, err := svc.Create(context.Background(), "https://example.com", "", 0)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if link.Code != "01234567" {
		t.Fatalf("code = %q, want 01234567", link.Code)
	}
}
