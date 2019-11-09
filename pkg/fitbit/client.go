package fitbit

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
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
	"profile",
	"heartrate",
}

type Client struct {
	Users        []*User
	oauthConfig  *oauth2.Config
	clientID     string
	clientSecret string
	db           *database.Database
}

type RequestError struct {
	Code       int
	RetryAfter time.Duration
	Errors     []struct {
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
		clientID:     clientID,
		clientSecret: clientSecret,
		db:           db,
		Users:        make([]*User, 0),
		oauthConfig:  defaultOauthConfig(clientID, clientSecret),
	}

	// Set a wait on the authenticated check
	if err := client.setupAuth(); err != nil {
		return nil, err
	}

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

func (c *Client) GetUser(userID string) (*User, error) {
	for _, u := range c.Users {
		if u.ID == userID {
			return u, nil
		}
	}

	return nil, fmt.Errorf("user %s does not exist", userID)
}

func (c *Client) setupAuth() error {
	users, err := c.getAllUsersWithTokens()
	if err != nil {
		return err
	}

	c.Users = users
	return nil
}

func (c *Client) LoginHandler(w http.ResponseWriter, r *http.Request) {
	url := c.oauthConfig.AuthCodeURL("")
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (c *Client) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")

	token, err := c.oauthConfig.Exchange(oauth2.NoContext, code)
	if err != nil {
		// If err getting a token then redirect to the root path to get a valid login token
		http.Redirect(w, r, listenURL, http.StatusTemporaryRedirect)
		return
	}

	httpClient := c.oauthConfig.Client(oauth2.NoContext, token)
	user, err := c.GetCurrentUser(httpClient)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}

	user.token = token
	user.httpClient = httpClient
	c.Users = append(c.Users, user)

	if err := user.save(c.db.GetDB()); err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	if err := user.saveToken(c.db.GetDB()); err != nil {
		w.Write([]byte(err.Error()))
		return
	}

	http.Redirect(w, r, "/"+user.ID, http.StatusTemporaryRedirect)
}

func (c *Client) saveTokens() error {
	for _, user := range c.Users {
		if err := user.saveToken(c.db.GetDB()); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Close() error {
	defer func() {
		for _, u := range c.Users {
			u.httpClient.CloseIdleConnections()
		}
	}()

	for _, user := range c.Users {
		// Get the current token and save it. This will help prevent a refreshed token from being missed
		oauthTransport, ok := user.httpClient.Transport.(*oauth2.Transport)
		if !ok {
			return nil
		}

		token, err := oauthTransport.Source.Token()
		if err != nil {
			return err
		}

		user.token = token
	}
	return c.saveTokens()
}

func (c *Client) get(client *http.Client, path string, output interface{}) error {
	resp, err := client.Get(path)
	if err != nil {
		return err
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode > 299 {
		errData := &RequestError{}
		if err := json.Unmarshal(b, errData); err != nil {
			return err
		}
		errData.Code = resp.StatusCode

		retryAfterStr := resp.Header.Get("Retry-After")
		if retryAfterStr != "" {
			retryAfter, err := strconv.Atoi(retryAfterStr)
			if err != nil {
				return err
			}
			errData.RetryAfter = time.Duration(retryAfter+30) * time.Second
		}

		return errData
	}

	if err := json.Unmarshal(b, output); err != nil {
		return err
	}

	return nil
}
