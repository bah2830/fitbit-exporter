package exporter

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
)

const fitbitCallsPerHour = 250

func (e *Exporter) backfill() error {
	log.Print("Starting fitbit data backfill...")

	// If backfill start not set a backfill will not be performed
	if e.cfg.Fitbit.BackfillStart == "" {
		return nil
	}

	startDate, err := time.Parse(dateFormat, e.cfg.Fitbit.BackfillStart)
	if err != nil {
		return err
	}

	// Get the most current date from the database
	var date string
	if err := e.db.GetDB().QueryRow("select date from heart_rest order by date DESC").Scan(&date); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	}
	if date != "" {
		lastDate, err := time.Parse(dateTimeFormat, date)
		if err != nil {
			return err
		}

		// Get the latest date from the backfill start and the last date in the database
		if lastDate.After(startDate) {
			startDate = lastDate
		}
	}

	log.Printf("Starting backfill from %s", startDate.Format(dateFormat))

	total := time.Since(startDate)
	days := total.Hours() / 24.0

	// With 250 api calls per hour and 1 api call per day get the total time required to make all calls
	backfillETA := time.Duration(int64(days/float64(fitbitCallsPerHour))*60) * time.Minute

	log.Printf("Backfill requires %.0f api calls with an ETA of %s", days, backfillETA.String())
	startTime := time.Now()

	// Loop through every day from the start date up until today.
	// Because of rate limits on the fitbit api we have to check for the too many calls response
	// When too many calls is hit we wait until the next hour mark for it reset and continue
	for date := startDate; !date.After(time.Now().UTC()); date = date.Add(24 * time.Hour) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
		d, err := e.getHeartData(ctx, date)
		if err != nil {
			return err
		}
		cancel()

		if err := e.client.SaveHeartRateData(d); err != nil {
			return err
		}
	}

	log.Printf("Backfill completed... completed in %s", time.Since(startTime).String())
	return nil
}

// getHeartData will attempt to get the heart rate data from the api
// repeatidly until it no longer has a rate limit error or the timeout occurs
func (e *Exporter) getHeartData(ctx context.Context, date time.Time) (*fitbit.HeartRateData, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting to get api data")
		default:
			d, err := e.client.GetHeartData(fitbit.HeartRateOptions{
				StartDate:   &date,
				EndDate:     &date,
				DetailLevel: fitbit.GetHeartRateDetailLevel(fitbit.HeartRateDetailLevel1Min),
			})
			if err != nil {
				if requestErr, ok := err.(*fitbit.RequestError); ok {
					// If this is a rate limit hit then just sleep until the hour is up and try again
					if requestErr.Code == http.StatusTooManyRequests {
						retryTime := time.Now().Add(requestErr.RetryAfter)
						log.Printf(
							"Rate limit hit, waiting %s and trying again (%s)",
							requestErr.RetryAfter.String(),
							retryTime.Format(dateTimeFormat),
						)
						time.Sleep(requestErr.RetryAfter)
						continue
					}

				}
				return nil, err
			}

			return d, nil
		}
	}
}
