package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/AVeukov/url-shortener/internal/cache"
	"github.com/AVeukov/url-shortener/internal/metrics"
	"github.com/AVeukov/url-shortener/internal/model"
	"github.com/AVeukov/url-shortener/internal/repository"
)

var (
	ErrInvalidURL   = errors.New("invalid URL")
	ErrInvalidAlias = errors.New("invalid custom alias")
	ErrAliasTaken   = errors.New("custom alias is already taken")
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,32}$`)

type LinkCache interface {
	Get(ctx context.Context, code string) (model.Link, error)
	Set(ctx context.Context, link model.Link) error
}

type Service struct {
	repository repository.LinkRepository
	cache      LinkCache
	metrics    *metrics.Metrics
	now        func() time.Time
	random     io.Reader
}

func New(repo repository.LinkRepository, linkCache LinkCache, m *metrics.Metrics) *Service {
	return &Service{repository: repo, cache: linkCache, metrics: m, now: time.Now, random: rand.Reader}
}

func (s *Service) Create(ctx context.Context, rawURL, customAlias string, lifetime time.Duration) (model.Link, error) {
	normalizedURL, err := validateURL(rawURL)
	if err != nil {
		return model.Link{}, err
	}

	createdAt := s.now().UTC()
	var expiresAt *time.Time
	if lifetime > 0 {
		expires := createdAt.Add(lifetime)
		expiresAt = &expires
	}

	if customAlias != "" {
		if !aliasPattern.MatchString(customAlias) || isReserved(customAlias) {
			return model.Link{}, ErrInvalidAlias
		}
		link := model.Link{Code: customAlias, OriginalURL: normalizedURL, CreatedAt: createdAt, ExpiresAt: expiresAt}
		if err := s.repository.Create(ctx, link); errors.Is(err, repository.ErrConflict) {
			return model.Link{}, ErrAliasTaken
		} else if err != nil {
			return model.Link{}, err
		}
		s.afterCreate(ctx, link)
		return link, nil
	}

	for range 5 {
		code, err := randomCode(s.random, 8)
		if err != nil {
			return model.Link{}, err
		}
		link := model.Link{Code: code, OriginalURL: normalizedURL, CreatedAt: createdAt, ExpiresAt: expiresAt}
		err = s.repository.Create(ctx, link)
		if errors.Is(err, repository.ErrConflict) {
			continue
		}
		if err != nil {
			return model.Link{}, err
		}
		s.afterCreate(ctx, link)
		return link, nil
	}
	return model.Link{}, errors.New("failed to generate a unique short code")
}

func (s *Service) Resolve(ctx context.Context, code string) (model.Link, error) {
	link, err := s.cache.Get(ctx, code)
	if err == nil {
		s.metrics.ObserveCache("hit")
		return link, nil
	}
	if errors.Is(err, cache.ErrMiss) {
		s.metrics.ObserveCache("miss")
	} else {
		s.metrics.ObserveCache("error")
	}

	link, err = s.repository.Get(ctx, code)
	if err != nil {
		return model.Link{}, err
	}
	_ = s.cache.Set(ctx, link)
	return link, nil
}

func (s *Service) Get(ctx context.Context, code string) (model.Link, error) {
	return s.repository.Get(ctx, code)
}

func (s *Service) RecordRedirect(ctx context.Context, code string) error {
	if err := s.repository.IncrementClicks(ctx, code); err != nil {
		return err
	}
	s.metrics.IncRedirects()
	return nil
}

func (s *Service) afterCreate(ctx context.Context, link model.Link) {
	s.metrics.IncLinksCreated()
	_ = s.cache.Set(ctx, link)
}

func validateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 4096 {
		return "", ErrInvalidURL
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ErrInvalidURL
	}
	return parsed.String(), nil
}

func randomCode(reader io.Reader, length int) (string, error) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	buffer := make([]byte, length)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	for i := range buffer {
		buffer[i] = alphabet[int(buffer[i])%len(alphabet)]
	}
	return string(buffer), nil
}

func isReserved(alias string) bool {
	switch alias {
	case "api", "metrics", "healthz", "readyz":
		return true
	default:
		return false
	}
}
