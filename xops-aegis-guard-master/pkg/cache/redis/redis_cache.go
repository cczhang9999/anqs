// Package redis implements the shared cache facade with go-redis.
package redis

import (
	"context"
	"errors"
	"time"

	"xops-aegis-guard/pkg/cache"
	jsonutil "xops-aegis-guard/util/json"

	redislib "github.com/redis/go-redis/v9"
)

const releaseScript = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`

var _ cache.Cacher = (*Cache)(nil)

// Clienter deliberately contains only operations used by the cache facade,
// which also makes the provider easy to substitute in tests.
type Clienter interface {
	Get(context.Context, string) *redislib.StringCmd
	Set(context.Context, string, interface{}, time.Duration) *redislib.StatusCmd
	SetNX(context.Context, string, interface{}, time.Duration) *redislib.BoolCmd
	Del(context.Context, ...string) *redislib.IntCmd
	Eval(context.Context, string, []string, ...interface{}) *redislib.Cmd
	Close() error
}

type Cache struct {
	cli Clienter
}

func NewCache(cli Clienter) *Cache { return &Cache{cli: cli} }

func (c *Cache) unavailable() bool { return c == nil || c.cli == nil }

func (c *Cache) Get(key string, ptrValue interface{}) error {
	if c.unavailable() {
		return cache.ErrUnavailable
	}
	data, err := c.cli.Get(context.Background(), key).Bytes()
	if errors.Is(err, redislib.Nil) {
		return cache.ErrCacheMissing
	}
	if err != nil {
		return err
	}
	return jsonutil.Unmarshal(data, ptrValue)
}

func (c *Cache) Set(key string, value interface{}, expiration time.Duration) error {
	if c.unavailable() {
		return cache.ErrUnavailable
	}
	data, err := jsonutil.Marshal(value)
	if err != nil {
		return err
	}
	return c.cli.Set(context.Background(), key, data, expiration).Err()
}

func (c *Cache) SetNX(key string, value interface{}, expiration time.Duration) (bool, error) {
	if c.unavailable() {
		return false, cache.ErrUnavailable
	}
	data, err := jsonutil.Marshal(value)
	if err != nil {
		return false, err
	}
	return c.cli.SetNX(context.Background(), key, data, expiration).Result()
}

func (c *Cache) Del(key ...string) (int64, error) {
	if c.unavailable() {
		return 0, cache.ErrUnavailable
	}
	return c.cli.Del(context.Background(), key...).Result()
}

func (c *Cache) CompareAndDelete(key string, value interface{}) (bool, error) {
	if c.unavailable() {
		return false, cache.ErrUnavailable
	}
	data, err := jsonutil.Marshal(value)
	if err != nil {
		return false, err
	}
	deleted, err := c.cli.Eval(context.Background(), releaseScript, []string{key}, data).Int64()
	if err != nil {
		return false, err
	}
	return deleted == 1, nil
}

func (c *Cache) Close() error {
	if c.unavailable() {
		return nil
	}
	return c.cli.Close()
}
