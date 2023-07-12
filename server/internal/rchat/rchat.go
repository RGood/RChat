package rchat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/RGood/rchat/server/internal/common"
	"github.com/RGood/rchat/server/internal/generated/chat"
	"google.golang.org/grpc/metadata"
)

// Server is the server struct for the rchat package
type Server struct {
	chat.UnimplementedRChatServer

	userService common.UserService

	// This should be a map that points to a sync set
	addressableEntity map[string]map[chat.RChat_OpenServer]struct{}
	sessions          map[string]string
}

func generateSecureToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("error generating token")
	}

	return hex.EncodeToString(b)
}

// NewServer is the server constructor function
func NewServer(userService common.UserService) chat.RChatServer {
	return &Server{
		userService:       userService,
		addressableEntity: map[string]map[chat.RChat_OpenServer]struct{}{},
		sessions:          map[string]string{},
	}
}

// Signup allows a new user to register for rchat
func (s *Server) Signup(ctx context.Context, creds *chat.Credentials) (*chat.AuthResponse, error) {
	// Attempt to create account
	err := s.userService.Create(creds.Username, creds.Password)
	if err != nil {
		return nil, err
	}

	token := generateSecureToken(256)
	s.sessions[token] = strings.ToLower(creds.Username)

	return &chat.AuthResponse{
		Token: token,
	}, nil
}

// Login allows a pre-existing to re-assume their unique identity
func (s *Server) Login(ctx context.Context, creds *chat.Credentials) (*chat.AuthResponse, error) {
	err := s.userService.Validate(creds.Username, creds.Password)
	if err != nil {
		return nil, err
	}

	token := generateSecureToken(256)
	s.sessions[token] = strings.ToLower(creds.Username)

	return &chat.AuthResponse{
		Token: token,
	}, nil
}

func (s *Server) getUserFromContext(ctx context.Context) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	vals := md.Get("token")
	if len(vals) > 0 {
		if username, ok := s.sessions[vals[0]]; ok {
			return username, nil
		}
	}

	return "", errors.New("session not found")
}

// Whoami returns the logged-in user's identity, derived from their auth token
func (s *Server) Whoami(ctx context.Context, req *chat.WhoamiRequest) (*chat.User, error) {
	// Check the metadata and return the corresponding user
	username, err := s.getUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	return &chat.User{Name: username}, nil
}

func deriveTargets(fullTarget string) (string, string) {
	targetSegments := strings.Split(fullTarget, "@")
	localTarget := strings.ToLower(targetSegments[0])
	newDest := strings.Join(targetSegments[1:], "@")

	return localTarget, newDest
}

// Open handles incoming connections
func (s *Server) Open(server chat.RChat_OpenServer) error {
	// Find user from context metadata in server.Context()
	// Register them in our map of addressable entities
	username, err := s.getUserFromContext(server.Context())
	if err != nil {
		return err
	}

	if targets, ok := s.addressableEntity[username]; ok {
		targets[server] = struct{}{}
	} else {
		s.addressableEntity[username] = map[chat.RChat_OpenServer]struct{}{
			server: {},
		}
	}

	defer func() {
		delete(s.addressableEntity[username], server)
		if len(s.addressableEntity[username]) == 0 {
			delete(s.addressableEntity, username)
		}
	}()

	// Loop listening for messages from the server & forward them to the appropriate address or respond with an error
	for event, err := server.Recv(); err != nil; event, err = server.Recv() {
		switch e := event.Event.(type) {
		case *chat.Event_Message:
			localTarget, newDest := deriveTargets(e.Message.Target)

			// If we can map the localTarget to a set of servers, send the message to them with updated address info
			if servers, ok := s.addressableEntity[localTarget]; ok {
				res := &chat.Event{
					Event: &chat.Event_Message{
						Message: &chat.Message{
							Target: newDest,
							Author: fmt.Sprintf("%s@%s", username, e.Message.Author),
							Time:   e.Message.Time,
							Data:   e.Message.Data,
						},
					},
				}

				for s := range servers {
					s.Send(res)
				}
			} else {
				// Otherwise return an error response
				server.Send(&chat.Event{
					Event: &chat.Event_Error{
						Error: &chat.ErrorResponse{
							Target: e.Message.Author,
							Event:  event,
						},
					},
				})

			}

		case *chat.Event_Error:
			localTarget, newDest := deriveTargets(e.Error.Target)

			if servers, ok := s.addressableEntity[localTarget]; ok {
				res := &chat.Event{
					Event: &chat.Event_Error{
						Error: &chat.ErrorResponse{
							Target: newDest,
							Event:  e.Error.Event,
						},
					},
				}

				for s := range servers {
					s.Send(res)
				}
			}
		}
	}

	return nil
}
