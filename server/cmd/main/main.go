package main

import (
	"net"

	"github.com/RGood/rchat/server/internal/generated/chat"
	"github.com/RGood/rchat/server/internal/rchat"
	userservice "github.com/RGood/rchat/server/internal/user_service"
	"google.golang.org/grpc"
)

func main() {
	userService, err := userservice.New("db", "postgres", "postgres", "postgres", 5432)
	if err != nil {
		panic(err)
	}

	rchat := rchat.NewServer(userService)

	lis, err := net.Listen("tcp", ":9000")
	if err != nil {
		panic(err)
	}

	srv := grpc.NewServer()
	srv.RegisterService(&chat.RChat_ServiceDesc, rchat)

	if err := srv.Serve(lis); err != nil {
		panic(err)
	}
}
