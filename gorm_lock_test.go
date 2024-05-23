package gormlock

import (
	"context"
	"github.com/go-co-op/gocron/v2"
	"gorm.io/gorm/logger"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testcontainerspostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestNewGormLocker_Validation(t *testing.T) {
	tests := map[string]struct {
		db     *gorm.DB
		worker string
		err    string
	}{
		"db is nil":       {db: nil, worker: "local", err: "gorm db definition can't be null"},
		"worker is empty": {db: &gorm.DB{}, worker: "", err: "worker name can't be null"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewGormLocker(tc.db, tc.worker)
			if assert.Error(t, err) {
				assert.ErrorContains(t, err, tc.err)
			}
		})
	}
}

func TestEnableDistributedLocking(t *testing.T) {
	ctx := context.Background()
	postgresContainer, err := testcontainerspostgres.RunContainer(ctx,
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(5*time.Second)))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	})

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable", "application_name=test")
	assert.NoError(t, err)

	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
	require.NoError(t, err)

	err = db.AutoMigrate(&CronJobLock{})
	require.NoError(t, err)

	resultChan := make(chan int, 10)
	f := func(schedulerInstance int) {
		resultChan <- schedulerInstance
		println(time.Now().Truncate(defaultPrecision).Format("2006-01-02 15:04:05.000"))
	}

	l1, err := NewGormLocker(db, "s1")
	require.NoError(t, err)
	s1, err := gocron.NewScheduler(
		gocron.WithLocation(time.UTC),
		gocron.WithDistributedLocker(l1))
	require.NoError(t, err)
	defer func() { _ = s1.Shutdown() }()

	job, err := s1.NewJob(
		gocron.DurationJob(time.Second),
		gocron.NewTask(f, 1))
	require.NoError(t, err)
	println("job created", job.ID().String())

	l2, err := NewGormLocker(db, "s2")
	require.NoError(t, err)
	s2, err := gocron.NewScheduler(
		gocron.WithLocation(time.UTC),
		gocron.WithDistributedLocker(l2))

	defer func() { _ = s2.Shutdown() }()

	job2, err := s2.NewJob(gocron.DurationJob(time.Second),
		gocron.NewTask(f, 2))
	require.NoError(t, err)
	println("job created", job2.ID().String())

	s1.Start()
	s2.Start()

	time.Sleep(3500 * time.Millisecond)

	_ = s1.StopJobs()
	_ = s2.StopJobs()

	close(resultChan)

	var results []int
	for r := range resultChan {
		results = append(results, r)
	}
	assert.Len(t, results, 3)
	var allCronJobs []*CronJobLock
	db.Find(&allCronJobs)
	assert.Equal(t, len(results), len(allCronJobs))
}

func TestEnableDistributedLocking_DifferentJob(t *testing.T) {
	ctx := context.Background()
	postgresContainer, err := testcontainerspostgres.RunContainer(ctx,
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(5*time.Second)))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	})

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable", "application_name=test")
	assert.NoError(t, err)

	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
	require.NoError(t, err)

	err = db.AutoMigrate(&CronJobLock{})
	require.NoError(t, err)

	resultChan := make(chan int, 10)
	f := func(schedulerInstance int) {
		println("f", strconv.FormatInt(int64(schedulerInstance), 10), time.Now().Truncate(defaultPrecision).Format("2006-01-02 15:04:05.000"))
		resultChan <- schedulerInstance
	}

	result2Chan := make(chan int, 10)
	f2 := func(schedulerInstance int) {
		println("f2", strconv.FormatInt(int64(schedulerInstance), 10), time.Now().Truncate(defaultPrecision).Format("2006-01-02 15:04:05.000"))
		result2Chan <- schedulerInstance
	}
	l1, err := NewGormLocker(db, "s1")
	s1, err := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l1))
	require.NoError(t, err)
	defer func() {
		_ = s1.Shutdown()
	}()

	job1, err := s1.NewJob(gocron.DurationJob(time.Second), gocron.NewTask(f, 1), gocron.WithName("f"))
	require.NoError(t, err)
	println("job created", job1.ID().String())
	job2, err := s1.NewJob(gocron.DurationJob(time.Second), gocron.NewTask(f2, 1), gocron.WithName("f2"))
	require.NoError(t, err)
	println("job created", job2.ID().String())

	l2, err := NewGormLocker(db, "s2")
	s2, err := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l2))
	require.NoError(t, err)
	defer func() {
		_ = s2.Shutdown()
	}()
	job3, err := s2.NewJob(gocron.DurationJob(time.Second), gocron.NewTask(f, 2), gocron.WithName("f"))
	require.NoError(t, err)
	println("job created", job3.ID().String())
	job4, err := s2.NewJob(gocron.DurationJob(time.Second), gocron.NewTask(f2, 2), gocron.WithName("f2"))
	require.NoError(t, err)
	println("job created", job4.ID().String())

	l3, err := NewGormLocker(db, "s3")
	s3, err := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l3))
	require.NoError(t, err)
	defer func() {
		_ = s3.Shutdown()
	}()

	job5, err := s3.NewJob(gocron.DurationJob(time.Second), gocron.NewTask(f, 3), gocron.WithName("f"))
	require.NoError(t, err)
	println("job created", job5.ID().String())

	job6, err := s3.NewJob(gocron.DurationJob(time.Second), gocron.NewTask(f2, 3), gocron.WithName("f2"))
	require.NoError(t, err)
	println("job created", job6.ID().String())

	s1.Start()
	s2.Start()
	s3.Start()

	time.Sleep(3500 * time.Millisecond)

	_ = s1.StopJobs()
	_ = s2.StopJobs()
	_ = s3.StopJobs()
	close(resultChan)
	close(result2Chan)

	var results []int
	for r := range resultChan {
		results = append(results, r)
	}
	assert.Len(t, results, 3, "f is expected 3 times")
	var results2 []int
	for r := range result2Chan {
		results2 = append(results2, r)
	}
	assert.Len(t, results2, 3, "f2 is expected 3 times")
	var allCronJobs []*CronJobLock
	db.Find(&allCronJobs)
	assert.Equal(t, len(results)+len(results2), len(allCronJobs))
}

func TestJobReturningExceptionWhenUnique(t *testing.T) {
	ctx := context.Background()
	postgresContainer, err := testcontainerspostgres.RunContainer(ctx,
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(5*time.Second)))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	})

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable", "application_name=test")
	assert.NoError(t, err)

	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
	require.NoError(t, err)

	err = db.AutoMigrate(&CronJobLock{})
	require.NoError(t, err)

	precision := 60 * time.Minute
	// creating an entry to force the unique identifier error
	cjb := &CronJobLock{
		JobName:       "job",
		JobIdentifier: time.Now().Truncate(precision).Format("2006-01-02 15:04:05.000"),
		Worker:        "local",
		Status:        StatusRunning,
	}
	require.NoError(t, db.Create(cjb).Error)

	l, _ := NewGormLocker(db, "local", WithDefaultJobIdentifier(precision))
	_, err = l.Lock(ctx, "job")
	if assert.Error(t, err) {
		assert.ErrorContains(t, err, "violates unique constraint")
	}
}

func TestHandleTTL(t *testing.T) {
	ctx := context.Background()
	postgresContainer, err := testcontainerspostgres.RunContainer(ctx,
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(5*time.Second)))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	})

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable", "application_name=test")
	assert.NoError(t, err)

	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&CronJobLock{})
	require.NoError(t, err)

	l1, err := NewGormLocker(db, "s1", WithTTL(1*time.Second))
	require.NoError(t, err)

	s1, err := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l1))

	job5, err := s1.NewJob(gocron.DurationJob(time.Second), gocron.NewTask(func() {}), gocron.WithName("f"))
	require.NoError(t, err)
	println("job created", job5.ID().String())

	s1.Start()

	time.Sleep(3500 * time.Millisecond)

	_ = s1.StopJobs()

	var allCronJobs []*CronJobLock
	db.Find(&allCronJobs)
	assert.GreaterOrEqual(t, len(allCronJobs), 3)

	// wait for data to expire
	time.Sleep(1500 * time.Millisecond)
	l1.cleanExpiredRecords()
	db.Find(&allCronJobs)
	assert.Equal(t, 0, len(allCronJobs))
}
