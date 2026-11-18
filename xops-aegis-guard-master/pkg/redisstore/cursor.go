package redisstore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"xops-aegis-guard/pkg/cache"
)

const (
	keyPrefix          = "xops-aegis-guard:alert-cursor:"
	notificationPrefix = "xops-aegis-guard:alert-notify:"
	assetRetryPrefix   = "xops-aegis-guard:alert-asset-retry:"
	lockKey            = "xops-aegis-guard:alert-lock"
	overlap            = 2 * time.Minute
	initialWindow      = 24 * time.Hour
	assetRetryTTL      = 24 * time.Hour
)

var ErrUnavailable = errors.New("redis cursor store is unavailable")

type Cursor struct {
	CursorTime    int64     `json:"cursor_time"`
	LastSuccessAt time.Time `json:"last_success_at"`
	LastPullCount int       `json:"last_pull_count"`
}

type Store struct{}

type assetRetryState struct {
	Count        int  `json:"count"`
	FallbackSent bool `json:"fallback_sent"`
}

func New() *Store { return &Store{} }

func (s *Store) Window(productType string, now time.Time) (start, end int64, found bool, err error) {
	if s == nil {
		return 0, 0, false, ErrUnavailable
	}
	end = now.UnixMilli()
	var cursor Cursor
	err = cache.Get(keyPrefix+productType, &cursor)
	if cache.IsMissing(err) {
		return now.Add(-initialWindow).UnixMilli(), end, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("read alert cursor %s: %w", productType, err)
	}
	if cursor.CursorTime <= 0 {
		return now.Add(-initialWindow).UnixMilli(), end, false, nil
	}
	return cursor.CursorTime - overlap.Milliseconds(), end, true, nil
}

func (s *Store) Commit(productType string, end int64, count int) error {
	if s == nil {
		return ErrUnavailable
	}
	cursor := Cursor{CursorTime: end, LastSuccessAt: time.Now(), LastPullCount: count}
	return cache.Set(keyPrefix+productType, cursor, 0)
}

// ClaimNotification atomically reserves a notification window. Force is used
// for new alerts and meaningful content changes; time-only refreshes use NX so
// multiple collectors and frequent provider updates cannot flood the robot.
func (s *Store) ClaimNotification(identity string, ttl time.Duration, force bool) (bool, error) {
	if s == nil {
		return false, ErrUnavailable
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return false, errors.New("notification identity is empty")
	}
	key := notificationPrefix + identity
	if force {
		if err := cache.Set(key, time.Now().Unix(), ttl); err != nil {
			return false, fmt.Errorf("mark alert notification: %w", err)
		}
		return true, nil
	}
	ok, err := cache.SetNX(key, time.Now().Unix(), ttl)
	if err != nil {
		return false, fmt.Errorf("claim alert notification: %w", err)
	}
	return ok, nil
}

// RecordIncidentAssetLookupFailure records a transient Cloud SIEM asset lookup
// failure. It returns true exactly once when the configured retry threshold is
// reached, allowing the caller to safely notify the unassigned robot.
func (s *Store) RecordIncidentAssetLookupFailure(identity string, threshold int) (count int, shouldFallback bool, err error) {
	if s == nil {
		return 0, false, ErrUnavailable
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return 0, false, errors.New("incident asset retry identity is empty")
	}
	if threshold <= 0 {
		threshold = 1
	}
	key := assetRetryPrefix + identity
	state := assetRetryState{}
	if err := cache.Get(key, &state); err != nil && !cache.IsMissing(err) {
		return 0, false, fmt.Errorf("read incident asset retry: %w", err)
	}
	state.Count++
	if state.Count >= threshold && !state.FallbackSent {
		state.FallbackSent = true
		shouldFallback = true
	}
	if err := cache.Set(key, state, assetRetryTTL); err != nil {
		return state.Count, false, fmt.Errorf("write incident asset retry: %w", err)
	}
	return state.Count, shouldFallback, nil
}

// ClearIncidentAssetLookupFailure removes a pending retry state after a later
// collection can associate the incident with a device. Its return value tells
// the caller whether a delayed, department-routed notification is required.
func (s *Store) ClearIncidentAssetLookupFailure(identity string) (bool, error) {
	if s == nil {
		return false, ErrUnavailable
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return false, errors.New("incident asset retry identity is empty")
	}
	key := assetRetryPrefix + identity
	var state assetRetryState
	err := cache.Get(key, &state)
	if cache.IsMissing(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read incident asset retry: %w", err)
	}
	if _, err := cache.Del(key); err != nil {
		return false, fmt.Errorf("clear incident asset retry: %w", err)
	}
	return state.Count > 0, nil
}

func (s *Store) AcquireLock(ttl time.Duration) (func(), error) {
	return s.AcquireNamedLock("alert", ttl)
}

func (s *Store) AcquireNamedLock(name string, ttl time.Duration) (func(), error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate redis lock token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	key := lockKey + ":" + name
	ok, err := cache.SetNX(key, token, ttl)
	if err != nil {
		return nil, fmt.Errorf("acquire alert lock: %w", err)
	}
	if !ok {
		return nil, errors.New("alert collection is already running")
	}
	return func() {
		_, _ = cache.CompareAndDelete(key, token)
	}, nil
}
