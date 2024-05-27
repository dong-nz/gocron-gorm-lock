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
	var nowFunc gormlock.NowFunc = func() time.Time {
		return time.Now().UTC()
	}
	connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d", "localhost", "postgres", "postgres", "demo", 5433)
	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{
		NowFunc: nowFunc,
		Logger:  logger.Default.LogMode(logger.Info)})
	
	// We need the table to store the job execution
	err = db.AutoMigrate(&gormlock.CronJobLock{})
	if err != nil {
		// handle the error
	}

	locker, err := gormlock.NewGormLocker(db, "w1",
		gormlock.WithDefaultJobIdentifier(nowFunc, time.Second),
	)
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
