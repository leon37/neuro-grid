package main

import (
	"log"
	"net"

	"github.com/leon37/neuro-grid/internal/server"
	"github.com/leon37/neuro-grid/pb"
	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	srv := server.NewNeuroServer()
	pb.RegisterNeuroServiceServer(s, srv)

	log.Println("Neuro-Grid Control Plane is ONLINE on :8080")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
