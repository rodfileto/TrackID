package infobio

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
)

type StoredSession struct {
	Cookies  map[string]string `json:"cookies"`
	Username string            `json:"username"`
}

type SessionStore interface {
	Save(context.Context, int64, StoredSession, time.Duration) error
	Load(context.Context, int64) (StoredSession, bool, error)
	Delete(context.Context, int64) error
}

type RedisSessionStore struct {
	pool   *redis.Pool
	prefix string
}

func NewRedisSessionStore(redisURL string) (*RedisSessionStore, error) {
	pool := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 5 * time.Minute,
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(redisURL)
		},
	}
	return &RedisSessionStore{pool: pool, prefix: "infobio:user:session:"}, nil
}

func (store *RedisSessionStore) key(userID int64) string {
	return store.prefix + strconv.FormatInt(userID, 10)
}

func (store *RedisSessionStore) Save(_ context.Context, userID int64, session StoredSession, ttl time.Duration) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	connection := store.pool.Get()
	defer connection.Close()
	_, err = connection.Do("SETEX", store.key(userID), int64(ttl/time.Second), payload)
	return err
}

func (store *RedisSessionStore) Load(_ context.Context, userID int64) (StoredSession, bool, error) {
	connection := store.pool.Get()
	defer connection.Close()
	payload, err := redis.Bytes(connection.Do("GET", store.key(userID)))
	if err == redis.ErrNil {
		return StoredSession{}, false, nil
	}
	if err != nil {
		return StoredSession{}, false, err
	}
	var session StoredSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return StoredSession{}, false, err
	}
	if len(session.Cookies) == 0 {
		return StoredSession{}, false, nil
	}
	return session, true, nil
}

func (store *RedisSessionStore) Delete(_ context.Context, userID int64) error {
	connection := store.pool.Get()
	defer connection.Close()
	_, err := connection.Do("DEL", store.key(userID))
	return err
}

func (store *RedisSessionStore) Close() error {
	return store.pool.Close()
}

type memoryEntry struct {
	session   StoredSession
	expiresAt time.Time
}

type MemorySessionStore struct {
	mu    sync.RWMutex
	data  map[int64]memoryEntry
	clock func() time.Time
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{data: map[int64]memoryEntry{}, clock: time.Now}
}

func (store *MemorySessionStore) Save(_ context.Context, userID int64, session StoredSession, ttl time.Duration) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.data[userID] = memoryEntry{session: session, expiresAt: store.clock().Add(ttl)}
	return nil
}

func (store *MemorySessionStore) Load(_ context.Context, userID int64) (StoredSession, bool, error) {
	store.mu.RLock()
	entry, ok := store.data[userID]
	store.mu.RUnlock()
	if !ok {
		return StoredSession{}, false, nil
	}
	if store.clock().After(entry.expiresAt) {
		store.mu.Lock()
		delete(store.data, userID)
		store.mu.Unlock()
		return StoredSession{}, false, nil
	}
	return entry.session, true, nil
}

func (store *MemorySessionStore) Delete(_ context.Context, userID int64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.data, userID)
	return nil
}
