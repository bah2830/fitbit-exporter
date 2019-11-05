package fitbit

type UserResponse struct {
	User *User `json:"user"`
}

type User struct {
	ID          string `json:"encodedId"`
	DisplayName string `json:"displayName"`
	FullName    string `json:"fullName"`
	MemberSince string `json:"memberSince"`
}

func (c *Client) GetCurrentUser() (*User, error) {
	user := &UserResponse{}
	if err := c.get(basePath+"/user/-/profile.json", user); err != nil {
		return nil, err
	}
	return user.User, nil
}
