package domain

type User struct {
	Username string `json:"username"`
}

type UserService interface {
	FindFriends(username string) ([]User, error)
}
