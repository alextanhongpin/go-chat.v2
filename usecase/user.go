package usecase

import "github.com/alextanhongpin/go-chat.v2/domain"

type UserService struct {
	users map[string][]domain.User
}

func NewUserService() *UserService {
	return &UserService{
		users: map[string][]domain.User{
			"john":  []domain.User{{Username: "alice"}, {Username: "bob"}},
			"alice": []domain.User{{Username: "john"}},
			"bob":   []domain.User{{Username: "john"}},
		},
	}
}

func (u *UserService) FindFriends(username string) ([]domain.User, error) {
	users := u.users[username]
	return users, nil
}
