package fitbit

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	heartRatePath = "/user/%s/activities/heart/date"

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
	Date        string
}

type HeartData struct {
	Time  string `json:"time"`
	Value int    `json:"value"`
}

func (c *Client) GetHeartData(user string, opts HeartRateOptions) (*HeartRateData, error) {
	path, err := opts.toPath(user)
	if err != nil {
		return nil, err
	}

	userClient, err := c.GetUser(user)
	if err != nil {
		return nil, err
	}

	data := &HeartRateData{}
	if err := c.get(userClient.httpClient, path, data); err != nil {
		return nil, err
	}

	return data, nil
}

func (o HeartRateOptions) toPath(user string) (string, error) {
	path := basePath + fmt.Sprintf(heartRatePath, user)

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

func (c *Client) SaveHeartRateData(user string, data *HeartRateData) error {
	db := c.db.GetDB()

	var day string
	for _, dayOverview := range data.OverviewByDay {
		day = dayOverview.Date

		// Save the resting heart rate if it hasn't been already
		var count int
		if err := db.QueryRow("select count(*) from heart_rest where user = ? and date = ?", user, day).Scan(&count); err != nil {
			return err
		}
		if count == 0 && dayOverview.Value.RestingHeartRate != 0 {
			_, err := db.Exec(
				"insert into heart_rest (user, date, value) values (?, ?, ?)",
				user,
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
				"select count(*) from heart_zone where user = ? and date = ? and type = ?",
				user,
				day,
				zone.Name,
			)
			if err := r.Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				_, err := db.Exec(
					"insert into heart_zone (user, date, type, minutes, calories) values (?, ?, ?, ?, ?)",
					user,
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

	// Get list of every existing datapoint on this day
	existingDates := make([]string, 0, 2000)
	rows, err := db.Query(
		"select date from heart_data where user = ? and date between ? and ?",
		user,
		day+" 00:00:00",
		day+" 23:59:59",
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return err
		}
		existingDates = append(existingDates, date)
	}

	// Breakup any intraday data into chunks of 200 to bulk insert
	var intradayChunks [][]string
	currentChunk := make([]string, 0, 200)

INTRA_LOOP:
	for i, d := range data.IntraDay.Data {
		if d.Value == 0 {
			continue
		}

		// Check if the date was alround found in the database
		for _, date := range existingDates {
			if date == day+" "+d.Time {
				continue INTRA_LOOP
			}
		}

		currentChunk = append(currentChunk, fmt.Sprintf("('%s', '%s', %d)", user, day+" "+d.Time, d.Value))
		if i != 0 && i%200 == 0 {
			intradayChunks = append(intradayChunks, currentChunk)
			currentChunk = make([]string, 0, 200)
		}
	}
	if len(currentChunk) > 0 {
		intradayChunks = append(intradayChunks, currentChunk)
	}

	// Insert 200 data points at a time to help take load off the database connection
	insertQuery := "insert into heart_data (user, date, value) values "
	for _, chunk := range intradayChunks {
		if _, err := db.Exec(insertQuery + strings.Join(chunk, ", ")); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) GetNHeartRates(user string, top bool, limit int) ([]HeartData, error) {
	order := "DESC"
	agg := "max"
	if !top {
		order = "ASC"
		agg = "min"
	}

	query := fmt.Sprintf(
		`select
			date,
			%s(value) as value
		from heart_data
		where user = '%s'
		group by DATE_FORMAT(date, '%%Y-%%m-%%d')
		order by %s(value) %s
		limit %d`,
		agg,
		user,
		agg,
		order,
		limit,
	)
	rows, err := c.db.GetDB().Query(query)
	if err != nil {
		return nil, err
	}

	results := make([]HeartData, 0, limit)
	for rows.Next() {
		var date string
		var value int

		if err := rows.Scan(&date, &value); err != nil {
			return nil, err
		}

		results = append(results, HeartData{
			Time:  date,
			Value: value,
		})
	}

	return results, nil
}

func (c *Client) GetResting(user string, top bool) (*HeartData, error) {
	order := "DESC"
	if !top {
		order = "ASC"
	}

	var date string
	var value int
	query := fmt.Sprintf("select DATE_FORMAT(date, '%%Y-%%m-%%d'), value from heart_rest where user = '%s' order by value %s", user, order)
	if err := c.db.GetDB().QueryRow(query).Scan(&date, &value); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &HeartData{
		Time:  date,
		Value: value,
	}, nil
}

func (c *Client) GetCurrentResting(user string) (int, error) {
	var value int
	query := "select value from heart_rest where user = ? and date_format(date, '%Y-%m-%d') = date_format(now(), '%Y-%m-%d')"
	if err := c.db.GetDB().QueryRow(query, user).Scan(&value, user); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	return value, nil
}

func (c *Client) GetCurrentDaysData(user string) ([]HeartData, error) {
	query := "select date, value from heart_data where user = ? and date_format(date, '%Y-%m-%d') = date_format(now(), '%Y-%m-%d')"
	rows, err := c.db.GetDB().Query(query, user)
	if err != nil {
		return nil, err
	}

	results := make([]HeartData, 0, 2000)
	for rows.Next() {
		var date string
		var value int
		if err := rows.Scan(&date, &value); err != nil {
			return nil, err
		}
		results = append(results, HeartData{
			Time:  date,
			Value: value,
		})
	}

	return results, nil
}

func (c *Client) GetCurrentDayLimit(user string, top bool) (*HeartData, error) {
	order := "DESC"
	if !top {
		order = "ASC"
	}

	query := fmt.Sprintf(
		"select date, value from heart_data where user = '%s' and date_format(date, '%%Y-%%m-%%d') = date_format(now(), '%%Y-%%m-%%d') order by value %s",
		user,
		order,
	)

	var date string
	var value int
	if err := c.db.GetDB().QueryRow(query).Scan(&date, &value); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &HeartData{
		Time:  date,
		Value: value,
	}, nil
}

func (c *Client) GetCurrentDayZones(user string) ([]HeartRateZone, error) {
	query := `select
		type,
		minutes,
		calories
	from heart_zone
	where
		user = ?
	and date_format(date, '%Y-%m-%d') = date_format(now(), '%Y-%m-%d')`

	rows, err := c.db.GetDB().Query(query, user)
	if err != nil {
		return nil, err
	}

	results := make([]HeartRateZone, 0, 4)
	for rows.Next() {
		var zoneType string
		var minutes, calories int
		if err := rows.Scan(&zoneType, &minutes, &calories); err != nil {
			return nil, err
		}

		results = append(results, HeartRateZone{
			Name:        zoneType,
			CaloriesOut: float64(calories),
			Minutes:     minutes,
		})

	}
	return results, nil
}

func (c *Client) GetZonesByDate(user string, startDate, endDate time.Time) ([]HeartRateZone, error) {
	query := `select
		type,
		minutes,
		calories
	from heart_zone
	where
		user = ?
	and date between ? and ?
	order by date, type`

	rows, err := c.db.GetDB().Query(query, user, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}

	results := make([]HeartRateZone, 0, 4)
	for rows.Next() {
		var zoneType string
		var minutes, calories int
		if err := rows.Scan(&zoneType, &minutes, &calories); err != nil {
			return nil, err
		}

		results = append(results, HeartRateZone{
			Name:        zoneType,
			CaloriesOut: float64(calories),
			Minutes:     minutes,
		})

	}
	return results, nil
}

func (c *Client) GetMaxZones(user string) (map[string]HeartRateZone, error) {
	query := `select
		date,
		type,
		minutes,
		calories
	from heart_zone
	where
		(user, type, minutes) in (
			select
				user,
				type,
				max(minutes)
			from heart_zone
			where user = ?
			group by type
		)`

	rows, err := c.db.GetDB().Query(query, user)
	if err != nil {
		return nil, err
	}

	results := make(map[string]HeartRateZone)
	for rows.Next() {
		var date, zoneType string
		var minutes, calories int
		if err := rows.Scan(&date, &zoneType, &minutes, &calories); err != nil {
			return nil, err
		}

		results[zoneType] = HeartRateZone{
			Date:        date,
			Name:        zoneType,
			CaloriesOut: float64(calories),
			Minutes:     minutes,
		}

	}
	return results, nil
}
