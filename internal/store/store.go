package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	_ "benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/sqlitedriver"
)

type Store struct {
	db    *sql.DB
	locks sync.Map
}

// ctxMutex 是一个支持上下文取消的互斥锁。
// 当调用者正在等待获取锁时，如果传入的 context 被取消，
// Lock 会立即返回 context 对应的错误而不是继续阻塞。
// 内部使用带缓冲的 channel 作为令牌：持有锁即拥有令牌，
// Unlock 将令牌放回 channel，等待者通过 select 竞争令牌或取消信号。
type ctxMutex struct {
	token chan struct{}
}

func newCtxMutex() *ctxMutex {
	m := &ctxMutex{token: make(chan struct{}, 1)}
	m.token <- struct{}{}
	return m
}

// Lock 尝试获取锁。如果 ctx 在获得锁之前被取消，返回 ctx.Err()。
// 获取锁成功后返回 nil。调用者必须在获取成功后调用 Unlock。
func (m *ctxMutex) Lock(ctx context.Context) error {
	select {
	case <-m.token:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Unlock 释放锁。
func (m *ctxMutex) Unlock() {
	m.token <- struct{}{}
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(8)
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接 SQLite: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) missionLock(id string) *ctxMutex {
	value, _ := s.locks.LoadOrStore(id, newCtxMutex())
	return value.(*ctxMutex)
}

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
