# gocron-gorm-lock
A gocron locker implementation using gorm

## install

```
go get github.com/dong-nz/gocron-gorm-lock
```

## usage

Here is an example usage that would be deployed in multiple instances

```go
package main

import (
	"fmt"
	gormlock "github.com/dong-nz/gocron-gorm-lock"
	"github.com/go-co-op/gocron/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

func main() {
	connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d", "localhost", "postgres", "postgres", "bifrost", 5432)
	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{Logger: logger.Default.LogMode(logger.Info), NowFunc: func() time.Time {
		return time.Now().UTC()
	}})

	// We need the table to store the job execution
	err = db.AutoMigrate(&gormlock.CronJobLock{})
	if err != nil {
		// handle the error
	}

	locker, err := gormlock.NewGormLocker(db, "w1")
	if err != nil {
		// handle the error
	}
	s, err := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(locker))
	if err != nil {
		// handle the error
	}
	defer func() { _ = s.Shutdown() }()

	job, err := s.NewJob(
		gocron.DurationJob(time.Second),
		gocron.NewTask(func() {
			// task to do
			fmt.Println("call 1s")
		}),
		gocron.WithName("job name"),
	)
	if err != nil {
		// handle the error
	}

	s.Start()

	run, err := job.NextRun()
	if err != nil {
		// handle the error
	}

	fmt.Println("next run: ", run)

	time.Sleep(3 * time.Second)

	err = s.StopJobs()
	if err != nil {
		// handle the error
	}

	var allCronJobs []*gormlock.CronJobLock
	db.Find(&allCronJobs)

	for _, cronJob := range allCronJobs {
		fmt.Println("cronJob: ", cronJob)
	}

	// just to see clean expired jobs executed by locker
	time.Sleep(5 * time.Second)
}


```

## Prerequisites

- The table cron_job_locks needs to exist in the database. This can be achieved, as an example, using gorm automigrate functionality `db.Automigrate(&CronJobLock{})`
- In order to uniquely identify the job, the locker uses the unique combination of the job name + timestamp (by default with precision to seconds).

## FAQ

- Q: The locker uses the unique combination of the job name + timestamp with seconds precision, how can I change that?
    - A: It's possible to change the timestamp precision used to uniquely identify the job, here is an example to set an hour precision:
      ```go
      locker, err := gormlock.NewGormLocker(db, "local", gormlock.WithDefaultJobIdentifier(60 * time.Minute))
      ```
- Q: But what about if we want to write our own implementation:
    - A: It's possible to set how to create the job identifier:
      ```go
      locker, err := gormlock.NewGormLocker(db, "local",
          gormlock.WithJobIdentifier(
              func(ctx context.Context, key string) string {
                  return ...
              },
          ),
      )
      ```