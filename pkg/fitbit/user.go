package fitbit

import "net/http"

type UserResponse struct {
	User *User `json:"user"`
}

type User struct {
	ID          string `json:"encodedId"`
	DisplayName string `json:"displayName"`
	FullName    string `json:"fullName"`
	MemberSince string `json:"memberSince"`
}

func (c *Client) GetCurrentUser(client *http.Client) (*User, error) {
	userResp := &UserResponse{}
	if err := c.get(client, basePath+"/user/-/profile.json", userResp); err != nil {
		return nil, err
	}
	return userResp.User, nil
}
