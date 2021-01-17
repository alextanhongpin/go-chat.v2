package usecase

type FriendService struct {
	friends map[string][]string
}

func NewFriendService() *FriendService {
	return &FriendService{
		friends: map[string][]string{
			"john":  []string{"alice", "bob"},
			"alice": []string{"john"},
			"bob":   []string{"john"},
		},
	}
}

func (f *FriendService) FindFriendsFor(user string) []string {
	return f.friends[user]
}
