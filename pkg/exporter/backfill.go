package exporter

import (
	"log"
	"net/http"
	"time"

	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
)

const fitbitCallsPerHour = 150

func (e *Exporter) backfill() error {
	log.Print("Starting fitbit data backfill...")
	startTime := time.Now()

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
		return err
	}
	lastDate, err := time.Parse(dateTimeFormat, date)
	if err != nil {
		return err
	}

	// Get the latest date from the backfill start and the last date in the database
	if lastDate.After(startDate) {
		startDate = lastDate
	}

	log.Printf("Starting backfill from %s", startDate.Format(dateFormat))

	total := time.Since(startDate)
	days := total.Hours() / 24.0

	// With 150 calls per day and 1 call per day get the hours required for all calls
	apiCallHours := days / float64(fitbitCallsPerHour)

	log.Printf("Backfill requires %.0f api calls with an ETA of %.2f hrs", days, apiCallHours)

	// Loop through every day from the start date up until today.
	// Because of rate limits on the fitbit api we have to check for the too many calls response
	// When too many calls is hit we wait until the next hour mark for it reset and continue
	for date := startDate; date.Before(time.Now().UTC()); date = date.Add(24 * time.Hour) {
		d, err := e.client.GetHeartData(fitbit.HeartRateOptions{
			StartDate:   &date,
			EndDate:     &date,
			DetailLevel: fitbit.GetHeartRateDetailLevel(fitbit.HeartRateDetailLevel1Min),
		})
		if err != nil {
			if requestErr, ok := err.(*fitbit.RequestError); ok {
				// If this is a rate limit hit then just sleep until hour is up and try again
				if requestErr.Code == http.StatusTooManyRequests {
					sleepUntil := time.Now().Truncate(time.Hour).Add(62 * time.Minute)
					sleepTime := sleepUntil.Sub(time.Now())
					log.Printf(
						"Rate limit hit, waiting until next hour to continue %s (%.0f min)",
						sleepUntil.Format(dateTimeFormat),
						sleepTime.Minutes(),
					)

					// Set the date back so the next call will repeat this one
					date = date.Add(-24 * time.Hour)

					time.Sleep(sleepTime)
					continue
				}

			}
			return err
		}

		if err := e.client.SaveHeartRateData(d); err != nil {
			return err
		}
	}

	log.Printf("Backfill completed... completed in %s", time.Since(startTime).String())
	return nil
}
