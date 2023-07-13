package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"syscall"

	"github.com/RGood/rchat/server/internal/generated/chat"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	waiting   int = 0
	messaging     = 1
)

func getUserCreds(verify bool) (string, string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return "", "", err
	}

	fmt.Print("Enter Password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", "", err
	}

	println()

	if verify {
		fmt.Print("Enter Password again: ")
		dupeBytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return "", "", err
		}

		println()
		if !reflect.DeepEqual(dupeBytePassword, bytePassword) {
			return "", "", errors.New("passwords did not match")
		}
	}

	password := string(bytePassword)
	return strings.TrimSpace(username), strings.TrimSpace(password), nil
}

func main() {
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	var signupFlag = flag.Bool("s", false, "Include this flag if you want to sign up instead of log in")
	var address = flag.String("a", ":443", "Set the target service address")
	flag.Parse()

	username, password, err := getUserCreds(*signupFlag)
	if err != nil {
		panic(err)
	}

	conn, err := grpc.Dial(*address, grpc.WithTransportCredentials(creds))
	if err != nil {
		panic(err)
	}

	client := chat.NewRChatClient(conn)
	var token string
	if *signupFlag {
		signupRes, err := client.Signup(context.Background(), &chat.Credentials{
			Username: username,
			Password: password,
		})
		if err != nil {
			panic(err)
		}
		token = signupRes.Token
	} else {
		signupRes, err := client.Login(context.Background(), &chat.Credentials{
			Username: username,
			Password: password,
		})
		if err != nil {
			panic(err)
		}
		token = signupRes.Token
	}

	md := metadata.New(map[string]string{
		"token": token,
	})

	ctx := metadata.NewOutgoingContext(context.Background(), md)

	res, err := client.Whoami(ctx, &chat.WhoamiRequest{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Logged in as: %s\n", res.Name)

	messageClient, err := client.Open(ctx)
	if err != nil {
		panic(err)
	}

	go func() {
		for event, err := messageClient.Recv(); err == nil; event, err = messageClient.Recv() {
			switch e := event.Event.(type) {
			case *chat.Event_Message:
				fmt.Printf("===============================\nAuthor: %s\n%s\n", e.Message.Author, e.Message.Data)
			case *chat.Event_Error:
				fmt.Printf("===============================\nEvent failed:\n%v\n", e.Error.Event)
			}
		}
	}()

	reader := bufio.NewReader(os.Stdin)

	mode := waiting
	activeTarget := ""
	for {
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if mode == waiting {
			parts := strings.Split(text, " ")
			command := parts[0]
			if command == "message" {
				activeTarget = parts[1]
				mode = messaging
			} else if command == "exit" {
				break
			} else {
				fmt.Printf("Invalid command: %s\n", command)
			}
		} else if mode == messaging {
			if text == "end" {
				mode = waiting
				activeTarget = ""
				println("Send closed.")
				continue
			}

			messageClient.Send(&chat.Event{
				Event: &chat.Event_Message{
					Message: &chat.Message{
						Target: activeTarget,
						Time:   timestamppb.Now(),
						Data:   text,
					},
				},
			})
		}
	}
}
