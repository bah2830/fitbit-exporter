package exporter

import (
	"time"

	"github.com/bah2830/fitbit-exporter/pkg/config"
	"github.com/bah2830/fitbit-exporter/pkg/database"
	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
)

const (
	dateFormat     = "2006-01-02"
	dateTimeFormat = "2006-01-02 15:04:05"
)

type Exporter struct {
	db              *database.Database
	client          *fitbit.Client
	cfg             *config.Config
	backfillRunning bool
	backfillLastRun time.Time
	user            *fitbit.User
}

func New(cfg *config.Config, client *fitbit.Client, db *database.Database) *Exporter {
	return &Exporter{
		cfg:    cfg,
		client: client,
		db:     db,
	}
}

func (e *Exporter) Start() error {
	if err := e.startFrontend(); err != nil {
		return err
	}

	// Wait for auth to occur before continuing
	e.client.WaitForAuth()

	user, err := e.client.GetCurrentUser()
	if err != nil {
		return err
	}
	e.user = user

	if err := e.startBackfiller(); err != nil {
		return err
	}

	// Run a backfill every hour to keep the most up to date data
	for range time.After(1 * time.Hour) {
		if err := e.startBackfiller(); err != nil {
			return err
		}
	}

	return nil
}

func (e *Exporter) Stop() error {
	return e.client.Close()
}

func (e *Exporter) startBackfiller() error {
	defer func() {
		e.backfillRunning = false
	}()

	e.backfillRunning = true
	e.backfillLastRun = time.Now()

	return e.backfill()
}
