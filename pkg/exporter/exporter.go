package exporter

import (
	"github.com/bah2830/fitbit-exporter/pkg/config"
	"github.com/bah2830/fitbit-exporter/pkg/database"
	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
)

const (
	dateFormat     = "2006-01-02"
	dateTimeFormat = "2006-01-02 15:04:05"
)

type Exporter struct {
	db     *database.Database
	client *fitbit.Client
	cfg    *config.Config
}

func New(cfg *config.Config, client *fitbit.Client, db *database.Database) *Exporter {
	return &Exporter{
		cfg:    cfg,
		client: client,
		db:     db,
	}
}

func (e *Exporter) Start() error {
	if err := e.client.StartAuthEndpoints(); err != nil {
		return err
	}

	return e.startBackfiller()
}

func (e *Exporter) Stop() error {
	return e.stopBackfiller()
}

func (e *Exporter) startBackfiller() error {
	// Wait for auth to occur before continuing
	e.client.WaitForAuth()
	return e.backfill()
}

func (e *Exporter) stopBackfiller() error {
	return nil
}
