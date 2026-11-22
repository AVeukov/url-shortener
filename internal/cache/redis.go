package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AVeukov/url-shortener/internal/model"
	"github.com/redis/go-redis/v9"
)

var ErrMiss = errors.New("cache miss")

type Redis struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedis(ctx context.Context, addr, password string, db int, ttl time.Duration) (*Redis, error) {
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Redis{client: client, ttl: ttl}, nil
}

func (r *Redis) Close() error { return r.client.Close() }

func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

func (r *Redis) Get(ctx context.Context, code string) (model.Link, error) {
	value, err := r.client.Get(ctx, key(code)).Bytes()
	if errors.Is(err, redis.Nil) {
		return model.Link{}, ErrMiss
	}
	if err != nil {
		return model.Link{}, fmt.Errorf("get cached link: %w", err)
	}
	var link model.Link
	if err := json.Unmarshal(value, &link); err != nil {
		return model.Link{}, fmt.Errorf("decode cached link: %w", err)
	}
	if link.Expired(time.Now()) {
		_ = r.Delete(ctx, code)
		return model.Link{}, ErrMiss
	}
	return link, nil
}

func (r *Redis) Set(ctx context.Context, link model.Link) error {
	ttl := r.ttl
	if link.ExpiresAt != nil {
		untilExpiry := time.Until(*link.ExpiresAt)
		if untilExpiry <= 0 {
			return nil
		}
		if untilExpiry < ttl {
			ttl = untilExpiry
		}
	}
	value, err := json.Marshal(link)
	if err != nil {
		return fmt.Errorf("encode cached link: %w", err)
	}
	if err := r.client.Set(ctx, key(link.Code), value, ttl).Err(); err != nil {
		return fmt.Errorf("cache link: %w", err)
	}
	return nil
}

func (r *Redis) Delete(ctx context.Context, code string) error {
	return r.client.Del(ctx, key(code)).Err()
}

func key(code string) string { return "short-link:" + code }
