package common

// UserService is the interface for managing user accounts
type UserService interface {
	Create(username, password string) error
	Validate(username, password string) error
}
