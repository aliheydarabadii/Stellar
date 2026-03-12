package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"api_gateway/internal/measurements/domain"
)

type Cache struct {
	client *redis.Client
}

func NewCache(ctx context.Context, address, username, password string, db int) (*Cache, error) {
	if strings.TrimSpace(address) == "" {
		return nil, errors.New("redis address is required")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Username: username,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Cache{client: client}, nil
}

func (c *Cache) Close() error {
	if c == nil || c.client == nil {
		return nil
	}

	return c.client.Close()
}

func (c *Cache) Ready(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("redis cache is not initialized")
	}

	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}

	return nil
}

func (c *Cache) Get(ctx context.Context, key string) (domain.MeasurementSeries, bool, error) {
	if c == nil || c.client == nil {
		return domain.MeasurementSeries{}, false, errors.New("redis cache is not initialized")
	}

	payload, err := c.client.Get(ctx, key).Bytes()
	switch {
	case errors.Is(err, redis.Nil):
		return domain.MeasurementSeries{}, false, nil
	case err != nil:
		return domain.MeasurementSeries{}, false, fmt.Errorf("redis get %q: %w", key, err)
	}

	var series domain.MeasurementSeries
	if err := json.Unmarshal(payload, &series); err != nil {
		return domain.MeasurementSeries{}, false, fmt.Errorf("decode cached response %q: %w", key, err)
	}

	return series, true, nil
}

func (c *Cache) Set(ctx context.Context, key string, value domain.MeasurementSeries, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return errors.New("redis cache is not initialized")
	}
	if ttl <= 0 {
		return errors.New("cache ttl must be positive")
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cached response %q: %w", key, err)
	}

	if err := c.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}

	return nil
}
