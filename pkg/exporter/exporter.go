package exporter

import (
	"github.com/bah2830/fitbit-exporter/pkg/config"
	"github.com/bah2830/fitbit-exporter/pkg/database"
	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
	"github.com/davecgh/go-spew/spew"
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

	e.startWorker()
	return nil
}

func (e *Exporter) Stop() error {
	return e.stopWorker()
}

func (e *Exporter) startWorker() error {
	// Wait for auth to occur before continuing
	e.client.WaitForAuth()

	d, err := e.client.GetHeartData(fitbit.HeartRateOptions{
		DetailLevel: fitbit.GetHeartRateDetailLevel(fitbit.HeartRateDetailLevel1Sec),
	})
	if err != nil {
		return err
	}

	spew.Dump(d)
	return nil
}

func (e *Exporter) stopWorker() error {
	return nil
}
