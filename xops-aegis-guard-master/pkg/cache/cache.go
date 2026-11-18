// Package cache provides the shared cache facade used by application code.
//
// Its layout follows xops-port-check/pkg/cache.  Keeping Redis behind this
// package prevents business packages from depending on go-redis directly.
package cache

import (
	"errors"
	"time"
)

const (
	ExpiresInOneMinute = time.Minute
	ExpiresInOneHour   = ExpiresInOneMinute * 60
	ExpiresInOneDay    = ExpiresInOneHour * 24
	ExpiresInAWeek     = ExpiresInOneDay * 7
	ExpiresInOneMonth  = ExpiresInOneDay * 30
)

var (
	// ErrCacheMissing means a cache key does not exist or has expired.
	ErrCacheMissing = errors.New("cache missing")
	// ErrUnavailable means Redis has not been configured or initialized.
	ErrUnavailable = errors.New("cache is unavailable")
)

// Cacher is the application cache contract. Values are JSON encoded by the
// provider, matching the cache convention in xops-port-check.
type Cacher interface {
	Get(key string, ptrValue interface{}) error
	Set(key string, value interface{}, expiration time.Duration) error
	SetNX(key string, value interface{}, expiration time.Duration) (bool, error)
	Del(key ...string) (int64, error)
	// CompareAndDelete deletes key only when its JSON-encoded value equals
	// value. It is used for distributed-lock release.
	CompareAndDelete(key string, value interface{}) (bool, error)
	Close() error
}

// Default is initialized during process bootstrap, after Redis connects.
var Default Cacher

func available() error {
	if Default == nil {
		return ErrUnavailable
	}
	return nil
}

func Get(key string, value interface{}) error {
	if err := available(); err != nil {
		return err
	}
	return Default.Get(key, value)
}

// Set stores a value. An expiration of zero means the value does not expire.
func Set(key string, value interface{}, expiration time.Duration) error {
	if err := available(); err != nil {
		return err
	}
	return Default.Set(key, value, expiration)
}

func SetNX(key string, value interface{}, expiration time.Duration) (bool, error) {
	if err := available(); err != nil {
		return false, err
	}
	return Default.SetNX(key, value, expiration)
}

func Del(key ...string) (int64, error) {
	if err := available(); err != nil {
		return 0, err
	}
	return Default.Del(key...)
}

func CompareAndDelete(key string, value interface{}) (bool, error) {
	if err := available(); err != nil {
		return false, err
	}
	return Default.CompareAndDelete(key, value)
}

func Close() error {
	if Default == nil {
		return nil
	}
	return Default.Close()
}

func IsMissing(err error) bool { return errors.Is(err, ErrCacheMissing) }

func IsUnavailable(err error) bool { return errors.Is(err, ErrUnavailable) }
