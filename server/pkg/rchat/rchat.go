package rchat

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/RGood/rchat/server/internal/generated/chat"
	"github.com/RGood/rchat/server/pkg/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type target interface {
	Send(*chat.Event) error
	Recv() (*chat.Event, error)
}

// Server is the server struct for the rchat package
type Server struct {
	chat.UnimplementedRChatServer

	userService common.UserService

	// This should be a map that points to a sync set
	addressableEntity map[string]map[target]struct{}
	sessions          map[string]string
}

// UpstreamServer is the config of our credentials to a remote service
type UpstreamServer struct {
	Username string
	Password string

	Address    string
	TargetName string
}

func generateSecureToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("error generating token")
	}

	return hex.EncodeToString(b)
}

// NewServer is the server constructor function
func NewServer(userService common.UserService, upstreams ...UpstreamServer) chat.RChatServer {
	server := &Server{
		userService:       userService,
		addressableEntity: map[string]map[target]struct{}{},
		sessions:          map[string]string{},
	}

	for _, upstream := range upstreams {
		if !validateUsername(upstream.TargetName) {
			continue
		}

		targetName := strings.ToLower(upstream.TargetName)

		server.addressableEntity[targetName] = map[target]struct{}{}

		creds := credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true,
		})

		conn, err := grpc.Dial(upstream.Address, grpc.WithTransportCredentials(creds))
		if err != nil {
			panic(err)
		}

		client := chat.NewRChatClient(conn)
		res, err := client.Login(context.Background(), &chat.Credentials{
			Username: upstream.Username,
			Password: upstream.Password,
		})

		// If we can't log into the remote service, skip
		if err != nil {
			continue
		}

		ctx := metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{
			"token": res.Token,
		}))

		upstreamClient, err := client.Open(ctx)
		if err != nil {
			continue
		}

		server.addressableEntity[targetName][upstreamClient] = struct{}{}
	}

	return server
}

// Define a global regular expression pattern
var alphanumericRegex = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func isAlphanumeric(s string) bool {
	return alphanumericRegex.MatchString(s)
}

func validateUsername(username string) bool {
	return len(username) >= 3 && len(username) <= 32 && isAlphanumeric(username)
}

// Signup allows a new user to register for rchat
func (s *Server) Signup(ctx context.Context, creds *chat.Credentials) (*chat.AuthResponse, error) {
	// Attempt to create account
	if !validateUsername(creds.Username) {
		return nil, errors.New("invalid username")
	}

	if _, ok := s.addressableEntity[strings.ToLower(creds.Username)]; ok {
		return nil, errors.New("entity name already exists")
	}

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
		s.addressableEntity[username] = map[target]struct{}{
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
	for event, err := server.Recv(); err == nil; event, err = server.Recv() {
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
