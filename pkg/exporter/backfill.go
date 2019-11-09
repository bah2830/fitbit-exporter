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
	startTime := time.Now()

	for _, user := range e.client.Users {
		startDate := time.Now()

		// Get the earliest date from the database
		var date string
		if err := e.db.GetDB().QueryRow("select date from heart_rest where user_id = ? order by date ASC", user.ID).Scan(&date); err != nil {
			if err != sql.ErrNoRows {
				return err
			}
		}
		if date != "" {
			earliestDate, err := time.Parse(dateTimeFormat, date)
			if err != nil {
				return err
			}

			if earliestDate.Before(startDate) {
				startDate = earliestDate
			}
		}

		log.Printf("Starting backfill for %s from %s", user.FullName, startDate.Format(dateFormat))

		// After 2 days of no data consider the backfill complete
		var daysWithoutData int

		// Loop through every day from the earliest date up until no more data is returned
		// Because of rate limits on the fitbit api we have to check for the too many calls response
		// When too many calls is hit we wait until the next hour mark for it reset and continue
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
			defer cancel()
			d, err := e.getHeartData(ctx, user.ID, startDate)
			if err != nil {
				return err
			}

			// If no intraday data found then we've hit the end of data available
			if (d.IntraDay == nil || len(d.IntraDay.Data) == 0) && len(d.OverviewByDay) == 0 {
				daysWithoutData++
				if daysWithoutData >= 2 {
					break
				}

			}

			if err := user.SaveHeartRateData(e.db.GetDB(), d); err != nil {
				return err
			}

			// Go back one day
			startDate = startDate.Add(-24 * time.Hour)
		}

		log.Printf(
			"Backfill completed for %s at %s... completed in %s",
			user.FullName,
			startDate.Format(dateFormat),
			time.Since(startTime).String(),
		)
	}

	log.Printf("Backfill completed... completed in %s", time.Since(startTime).String())
	return nil
}

// getHeartData will attempt to get the heart rate data from the api
// repeatidly until it no longer has a rate limit error or the timeout occurs
func (e *Exporter) getHeartData(ctx context.Context, user string, date time.Time) (*fitbit.HeartRateData, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting to get api data")
		default:
			d, err := e.client.GetHeartData(user, fitbit.HeartRateOptions{
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
							"Rate limit hit while at %s, waiting %s and trying again (%s)",
							date.Format(dateFormat),
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
