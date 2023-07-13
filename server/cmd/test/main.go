package main

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/RGood/rchat/server/internal/generated/chat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

func main() {
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	conn, err := grpc.Dial(":1443", grpc.WithTransportCredentials(creds))
	if err != nil {
		panic(err)
	}

	client := chat.NewRChatClient(conn)

	// signupRes, err := client.Signup(context.Background(), &chat.Credentials{
	// 	Username: "foo",
	// 	Password: "bar",
	// })
	signupRes, err := client.Login(context.Background(), &chat.Credentials{
		Username: "foo",
		Password: "bar",
	})
	if err != nil {
		panic(err)
	}

	md := metadata.New(map[string]string{
		"token": signupRes.Token,
	})

	ctx := metadata.NewOutgoingContext(context.Background(), md)

	res, err := client.Whoami(ctx, &chat.WhoamiRequest{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("User: %s\n", res.Name)
}
