package fitbit

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bah2830/fitbit-exporter/pkg/database"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/fitbit"
)

const (
	callbackURL = "http://localhost:3000/callback"
	listenURL   = "http://localhost:3000/"
	basePath    = "https://api.fitbit.com/1"
)

var requiredScopes = []string{
	"heartrate",
}

type Client struct {
	httpClient    *http.Client
	token         *oauth2.Token
	clientID      string
	clientSecret  string
	db            *database.Database
	authenticated *sync.WaitGroup
}

type RequestError struct {
	Code   int
	Errors []struct {
		ErrorType string `json:"errorType"`
		FieldName string `json:"fieldName"`
		Message   string `json:"message"`
	} `json:"errors"`
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("ERROR (%d): %s", e.Code, e.string())
}

func (e *RequestError) string() string {
	var msg string
	for _, m := range e.Errors {
		msg += m.Message + " "
	}
	return strings.TrimSpace(msg)
}

func NewClient(db *database.Database, clientID, clientSecret string) (*Client, error) {
	client := &Client{
		clientID:      clientID,
		clientSecret:  clientSecret,
		authenticated: &sync.WaitGroup{},
		db:            db,
	}

	// Set a wait on the authenticated check
	client.authenticated.Add(1)

	return client, nil
}

func defaultOauthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  callbackURL,
		Endpoint:     fitbit.Endpoint,
		Scopes:       requiredScopes,
	}
}

func (c *Client) StartAuthEndpoints() error {
	token, err := c.getPreviousToken()
	if err != nil {
		return err
	}

	// If a previous token was found and it hasn't expired then use it rather than wait for auth
	if token != nil && token.Expiry.After(time.Now()) {
		c.token = token
		c.httpClient = defaultOauthConfig(c.clientID, c.clientSecret).Client(oauth2.NoContext, c.token)
		c.authenticated.Done()
	}

	config := defaultOauthConfig(c.clientID, c.clientSecret)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		url := config.AuthCodeURL("")
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	})

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if c.token == nil {
			code := r.FormValue("code")

			var err error
			c.token, err = config.Exchange(oauth2.NoContext, code)
			if err != nil {
				// If err getting a token then redirect to the root path to get a valid login token
				http.Redirect(w, r, listenURL, http.StatusTemporaryRedirect)
				return
			}

			// Setup the http client
			c.httpClient = defaultOauthConfig(c.clientID, c.clientSecret).Client(oauth2.NoContext, c.token)
			c.authenticated.Done()
			if err := c.saveToken(); err != nil {
				w.Write([]byte(err.Error()))
				return
			}
		}

		w.Write([]byte("Login Successful"))
	})

	log.Println("listening on localhost:3000")
	go http.ListenAndServe(":3000", nil)
	return nil
}

func (c *Client) WaitForAuth() {
	if c.token != nil {
		return
	}

	fmt.Printf("Waiting for authentication to complete (%s)..\n", listenURL)
	c.authenticated.Wait()
}

func (c *Client) getPreviousToken() (*oauth2.Token, error) {
	var accessToken, refreshToken, tokenType, expiration string
	err := c.db.GetDB().
		QueryRow("select access_token, refresh_token, token_type, expiration from fitbit_token").
		Scan(&accessToken, &refreshToken, &tokenType, &expiration)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
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

	return token, nil
}

func (c *Client) saveToken() error {
	if c.token == nil {
		return nil
	}

	// Delete all previos tokens
	if _, err := c.db.GetDB().Exec("delete from fitbit_token"); err != nil {
		return err
	}

	insertStatement := `insert into fitbit_token
		(access_token, refresh_token, token_type, expiration)
		values (?, ?, ?, ?)`

	_, err := c.db.GetDB().Exec(
		insertStatement,
		c.token.AccessToken,
		c.token.RefreshToken,
		c.token.TokenType,
		c.token.Expiry.Format(database.DateTimeFormat),
	)
	if err != nil {
		return err
	}

	return nil
}
