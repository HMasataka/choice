package store

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// GoRedisClient is an adapter for go-redis/v9 that implements the RedisClient interface.
type GoRedisClient struct {
	client *redis.Client
}

// NewGoRedisClient creates a new GoRedisClient with the provided options.
func NewGoRedisClient(opts *redis.Options) (*GoRedisClient, error) {
	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &GoRedisClient{client: client}, nil
}

// Set sets a key-value pair with an expiration time.
func (c *GoRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.client.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value by key.
func (c *GoRedisClient) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// Del deletes one or more keys.
func (c *GoRedisClient) Del(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

// SAdd adds one or more members to a set.
func (c *GoRedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SAdd(ctx, key, members...).Err()
}

// SMembers retrieves all members of a set.
func (c *GoRedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

// SRem removes one or more members from a set.
func (c *GoRedisClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SRem(ctx, key, members...).Err()
}

// Expire sets a timeout on a key.
func (c *GoRedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.client.Expire(ctx, key, expiration).Err()
}

// Exists checks if keys exist.
func (c *GoRedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Exists(ctx, keys...).Result()
}

// HSet sets hash field-value pairs.
func (c *GoRedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	return c.client.HSet(ctx, key, values...).Err()
}

// HGet retrieves a hash field value.
func (c *GoRedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	val, err := c.client.HGet(ctx, key, field).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// HGetAll retrieves all hash fields and values.
func (c *GoRedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

// HIncrBy increments a hash field by an integer.
func (c *GoRedisClient) HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error) {
	return c.client.HIncrBy(ctx, key, field, incr).Result()
}

// Keys returns all keys matching a pattern.
func (c *GoRedisClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	return c.client.Keys(ctx, pattern).Result()
}

// Close closes the Redis client connection.
func (c *GoRedisClient) Close() error {
	return c.client.Close()
}
