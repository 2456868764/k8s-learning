package main

import (
	"net"

	pb "github.com/2456868764/k8s-learning/grpc/basic/ecommerce"
	"google.golang.org/grpc"
)

func main() {
	s := grpc.NewServer()

	pb.RegisterOrderManagementServer(s, &OrderManagementImpl{})

	lit, err := net.Listen("tcp", ":8009")
	if err != nil {
		panic(err)
	}

	if err := s.Serve(lit); err != nil {
		panic(err)
	}
}
