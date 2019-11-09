package fitbit

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bah2830/fitbit-exporter/pkg/database"
	"golang.org/x/oauth2"
)

type UserResponse struct {
	User *User `json:"user"`
}

type User struct {
	ID          string `json:"encodedId"`
	DisplayName string `json:"displayName"`
	FullName    string `json:"fullName"`
	MemberSince string `json:"memberSince"`

	token      *oauth2.Token
	httpClient *http.Client
}

func (c *Client) GetCurrentUser(client *http.Client) (*User, error) {
	userResp := &UserResponse{}
	if err := c.get(client, basePath+"/user/-/profile.json", userResp); err != nil {
		return nil, err
	}
	return userResp.User, nil
}

func (u *User) save(db *sql.DB) error {
	// Check if user exists
	var count int
	if err := db.QueryRow("select count(id) from user where id = ?", u.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// Insert the new user
	if _, err := db.Exec(
		"insert into user (id, full_name, display_name, member_since) values (?, ?, ?, ?)",
		u.ID,
		u.FullName,
		u.DisplayName,
		u.MemberSince,
	); err != nil {
		return err
	}
	return nil
}

func (c *Client) getAllUsersWithTokens() ([]*User, error) {
	rows, err := c.db.GetDB().Query(`
		select
			u.id,
			u.full_name,
			u.display_name,
			u.member_since,
			t.access_token,
			t.refresh_token,
			t.token_type,
			t.expiration
		from
			user u
		join user_token t on t.user_id = u.id`,
	)
	if err != nil {
		return nil, err
	}

	users := make([]*User, 0)
	for rows.Next() {
		var userID, user, displayName, memberSince, accessToken, refreshToken, tokenType, expiration string
		if err := rows.Scan(&userID, &user, &displayName, &memberSince, &accessToken, &refreshToken, &tokenType, &expiration); err != nil {
			return nil, err
		}

		expiry, err := time.Parse(database.DateTimeFormat, expiration)
		if err != nil {
			return nil, err
		}

		token := &oauth2.Token{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			TokenType:    tokenType,
			Expiry:       expiry,
		}

		// If a previous token was found and it hasn't expired then use it rather than wait for auth
		users = append(users, &User{
			ID:          userID,
			FullName:    user,
			DisplayName: displayName,
			MemberSince: memberSince,
			token:       token,
			httpClient:  c.oauthConfig.Client(oauth2.NoContext, token),
		})
	}

	return users, nil
}

func (u *User) saveToken(db *sql.DB) error {
	// Delete all previous tokens for the user
	if _, err := db.Exec("delete from user_token where user_id = ?", u.ID); err != nil {
		return err
	}

	insertStatement := `insert into user_token
	(user_id, access_token, refresh_token, token_type, expiration)
	values (?, ?, ?, ?, ?)`

	_, err := db.Exec(
		insertStatement,
		u.ID,
		u.token.AccessToken,
		u.token.RefreshToken,
		u.token.TokenType,
		u.token.Expiry.Format(database.DateTimeFormat),
	)
	return err
}

func (u *User) SaveHeartRateData(db *sql.DB, data *HeartRateData) error {
	var day string
	for _, dayOverview := range data.OverviewByDay {
		day = dayOverview.Date

		// Save the resting heart rate if it hasn't been already
		var count int
		if err := db.QueryRow("select count(*) from heart_rest where user_id = ? and date = ?", u.ID, day).Scan(&count); err != nil {
			return err
		}
		if count == 0 && dayOverview.Value.RestingHeartRate != 0 {
			_, err := db.Exec(
				"insert into heart_rest (user_id, date, value) values (?, ?, ?)",
				u.ID,
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
				"select count(*) from heart_zone where user_id = ? and date = ? and type = ?",
				u.ID,
				day,
				zone.Name,
			)
			if err := r.Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				_, err := db.Exec(
					"insert into heart_zone (user_id, date, type, minutes, calories) values (?, ?, ?, ?, ?)",
					u.ID,
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
		"select date from heart_data where user_id = ? and date between ? and ?",
		u.ID,
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

		currentChunk = append(currentChunk, fmt.Sprintf("('%s', '%s', %d)", u.ID, day+" "+d.Time, d.Value))
		if i != 0 && i%200 == 0 {
			intradayChunks = append(intradayChunks, currentChunk)
			currentChunk = make([]string, 0, 200)
		}
	}
	if len(currentChunk) > 0 {
		intradayChunks = append(intradayChunks, currentChunk)
	}

	// Insert 200 data points at a time to help take load off the database connection
	insertQuery := "insert into heart_data (user_id, date, value) values "
	for _, chunk := range intradayChunks {
		if _, err := db.Exec(insertQuery + strings.Join(chunk, ", ")); err != nil {
			return err
		}
	}

	return nil
}
