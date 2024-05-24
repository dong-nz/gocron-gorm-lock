package gormlock

import (
	"context"
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

type NowFunc func() time.Time

var (
	defaultPrecision     = time.Second
	defaultJobIdentifier = func(nowFunc NowFunc, precision time.Duration) func(ctx context.Context, key string) string {
		return func(ctx context.Context, key string) string {
			return nowFunc().Truncate(precision).Format("2006-01-02 15:04:05.000")
		}
	}

	defaultTTL                   = 24 * time.Hour
	defaultCleanInterval         = 5 * time.Second
	defaultNowFunc       NowFunc = func() time.Time {
		return time.Now().UTC()
	}
)

type JobLockStatus string

const (
	StatusRunning  JobLockStatus = "RUNNING"
	StatusFinished JobLockStatus = "FINISHED"
)

func NewGormLocker(db *gorm.DB, worker string, options ...LockOption) (*GormLocker, error) {
	if db == nil {
		return nil, fmt.Errorf("gorm db definition can't be null")
	}
	if worker == "" {
		return nil, fmt.Errorf("worker name can't be null")
	}

	gl := &GormLocker{
		db:       db,
		worker:   worker,
		ttl:      defaultTTL,
		interval: defaultCleanInterval,
		nowFunc:  defaultNowFunc,
	}

	for _, option := range options {
		option(gl)
	}

	// make sure job identifier function uses nowFunc the same as gorm config
	if gl.jobIdentifier == nil {
		gl.jobIdentifier = defaultJobIdentifier(gl.nowFunc, defaultPrecision)
	}

	go func() {
		ticker := time.NewTicker(gl.interval)
		defer ticker.Stop()

		for range ticker.C {
			if gl.closed.Load() {
				return
			}

			gl.cleanExpiredRecords()
		}
	}()

	return gl, nil
}

var _ gocron.Locker = (*GormLocker)(nil)

type GormLocker struct {
	db            *gorm.DB
	worker        string
	ttl           time.Duration
	interval      time.Duration
	jobIdentifier func(ctx context.Context, key string) string
	nowFunc       func() time.Time

	closed atomic.Bool
}

func (g *GormLocker) cleanExpiredRecords() {
	g.db.Unscoped().Where("updated_at < ? and status = ?", g.nowFunc().Add(-g.ttl).UnixMilli(), StatusFinished).Delete(&CronJobLock{})
}

func (g *GormLocker) Close() {
	g.closed.Store(true)
}

func (g *GormLocker) Lock(ctx context.Context, key string) (gocron.Lock, error) {
	ji := g.jobIdentifier(ctx, key)

	// I would like that people can "pass" their own implementation,
	cjb := &CronJobLock{
		JobName:       key,
		JobIdentifier: ji,
		Worker:        g.worker,
		Status:        StatusRunning,
	}
	tx := g.db.Create(cjb)
	if tx.Error != nil {
		return nil, tx.Error
	}
	return &gormLock{db: g.db, id: cjb.GetID()}, nil
}

var _ gocron.Lock = (*gormLock)(nil)

type gormLock struct {
	db *gorm.DB
	id uint64
}

func (g *gormLock) Unlock(_ context.Context) error {
	return g.db.Model(&CronJobLock{ID: g.id}).Updates(&CronJobLock{Status: StatusFinished}).Error
}
