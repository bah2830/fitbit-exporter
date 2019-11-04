package fitbit

import (
	"encoding/json"
	"io/ioutil"
	"time"
)

const (
	heartRatePath = "/user/-/activities/heart/date"

	HeartRateDetailLevel1Sec HeartRateDetailLevel = "1sec"
	HeartRateDetailLevel1Min HeartRateDetailLevel = "1min"
)

type HeartRateDetailLevel string
type HeartRatePeriod string

type HeartRateOptions struct {
	StartDate   *time.Time
	EndDate     *time.Time
	DetailLevel *HeartRateDetailLevel
}

type HeartRateData struct {
	OverviewByDay []HeartRateOverView `json:"activities-heart"`
	IntraDay      *HeartRateIntraDay  `json:"activities-heart-intraday"`
}

type HeartRateOverView struct {
	Date  string                 `json:"dateTime"`
	Value HeartRateOverviewValue `json:"value"`
}

type HeartRateOverviewValue struct {
	Zones            []HeartRateZone `json:"heartRateZones"`
	RestingHeartRate int             `json:"restingHeartRate"`
}

type HeartRateIntraDay struct {
	Data             []HeartData `json:"dataset"`
	DataInterval     int         `json:"datasetInterval"`
	DataIntervalType string      `json:"datasetType"`
}

type HeartRateZone struct {
	Name        string  `json:"name"`
	CaloriesOut float64 `json:"caloriesOut"`
	Max         int     `json:"max"`
	Min         int     `json:"Min"`
	Minutes     int     `json:"minutes"`
}

type HeartData struct {
	Time  string `json:"time"`
	Value int    `json:"value"`
}

func (c *Client) GetHeartData(opts HeartRateOptions) (*HeartRateData, error) {
	path, err := opts.toPath()
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Get(path)
	if err != nil {
		return nil, err
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode > 299 {
		errData := &RequestError{}
		if err := json.Unmarshal(b, errData); err != nil {
			return nil, err
		}
		errData.Code = resp.StatusCode

		return nil, errData
	}

	data := &HeartRateData{}
	if err := json.Unmarshal(b, data); err != nil {
		return nil, err
	}

	return data, nil
}

func (o HeartRateOptions) toPath() (string, error) {
	path := basePath + heartRatePath

	if o.StartDate == nil {
		path += "/today"
	} else {
		path += "/" + o.StartDate.Format("2006-01-02")
	}

	// If end date not given then default to 1 day from start
	if o.EndDate == nil {
		if o.StartDate == nil {
			path += "/1d"
		} else {
			path += "/" + o.StartDate.Add(24*time.Hour).Format("2006-01-02")
		}
	} else {
		path += "/" + o.EndDate.Format("2006-01-02")
	}

	if o.DetailLevel != nil {
		path += "/" + string(*o.DetailLevel)
	}

	path += ".json"

	return path, nil
}

func GetHeartRateDetailLevel(level HeartRateDetailLevel) *HeartRateDetailLevel {
	return &level
}

func GetHeartRatePeriod(period HeartRatePeriod) *HeartRatePeriod {
	return &period
}

func (c *Client) SaveHeartRateData(data *HeartRateData) error {
	db := c.db.GetDB()

	var day string
	for _, dayOverview := range data.OverviewByDay {
		day = dayOverview.Date

		// Save the resting heart rate if it hasn't been already
		var count int
		if err := db.QueryRow("select count(*) from heart_rest where date = ?", day).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			_, err := db.Exec(
				"insert into heart_rest (date, value) values (?, ?)",
				day,
				dayOverview.Value.RestingHeartRate,
			)
			if err != nil {
				return err
			}
		}

		// Save the zone data if not already exists
		for _, zone := range dayOverview.Value.Zones {
			var count int
			r := db.QueryRow(
				"select count(*) from heart_zone where date = ? and type = ?",
				day,
				zone.Name,
			)
			if err := r.Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				_, err := db.Exec(
					"insert into heart_zone (date, type, minutes, calories) values (?, ?, ?, ?)",
					day,
					zone.Name,
					zone.Minutes,
					zone.CaloriesOut,
				)
				if err != nil {
					return err
				}
			}
		}
	}

	// Save the intraday data if not already exists
	for _, d := range data.IntraDay.Data {
		// Check to see if the data already exists
		var count int
		dateTime := day + " " + d.Time
		if err := db.QueryRow("select count(*) from heart_data where date = ?", dateTime).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		_, err := db.Exec(
			"insert into heart_data (date, value) values (?, ?)",
			dateTime,
			d.Value,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
