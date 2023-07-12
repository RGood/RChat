package userservice

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Service is a struct for managing user transactions with Postgres
type Service struct {
	db *gorm.DB
}

// Account is the struct representing a user account
type user struct {
	Username string
	Password []byte
	Salt     string
}

// Verify determines if the password is correct for an account
func (acc *user) verify(password string) bool {
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", acc.Salt, password)))

	pw := hash.Sum(nil)

	return reflect.DeepEqual(pw, acc.Password)
}

// New instantiates an instance of the service struct
func New(host, user, password, dbname string, port int) (*Service, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d", host, user, password, dbname, port)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})

	if err != nil {
		return nil, err
	}

	return &Service{
		db: db,
	}, nil
}

// Create creates a new user account
func (s *Service) Create(username, password string) error {
	salt := randStringRunes(256)
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", salt, password)))

	pw := hash.Sum(nil)

	acc := &user{
		Username: strings.ToLower(username),
		Password: pw,
		Salt:     salt,
	}

	result := s.db.Create(acc)
	return result.Error
}

// Validate checks if a username/password pair are valid
func (s *Service) Validate(username, password string) error {
	acc := &user{Username: strings.ToLower(username)}
	s.db.First(acc)

	if acc.verify(password) {
		return nil
	}

	return errors.New("invalid username/password")
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
