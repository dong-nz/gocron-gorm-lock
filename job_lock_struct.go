package gormlock

import "time"

type JobLock[T any] interface {
	GetID() T
	SetJobIdentifier(ji string)
}

var _ JobLock[uint64] = (*CronJobLock)(nil)

type CronJobLock struct {
	ID            uint64        `gorm:"primaryKey,autoIncrement"`
	CreatedAt     int64         `gorm:"autoCreateTime:milli"`
	UpdatedAt     int64         `gorm:"autoUpdateTime:milli"`
	JobName       string        `gorm:"size:256;index:idx_name,unique"`
	JobIdentifier string        `gorm:"size:256;index:idx_name,unique"`
	Worker        string        `gorm:"size:256;not null"`
	Status        JobLockStatus `gorm:"size:10;not null"`
}

func (cjb *CronJobLock) GetCreateAt() time.Time {
	return time.UnixMilli(cjb.CreatedAt)
}

func (cjb *CronJobLock) GetUpdateAt() time.Time {
	return time.UnixMilli(cjb.UpdatedAt)
}

func (cjb *CronJobLock) SetJobIdentifier(ji string) {
	cjb.JobIdentifier = ji
}

func (cjb *CronJobLock) GetID() uint64 {
	return cjb.ID
}
