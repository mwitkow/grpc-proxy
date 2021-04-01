package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/mwitkow/grpc-proxy/testservice"
)

var (
	port = flag.Uint("port", 8080, "Port to listen to")
)

func main() {
	srv := grpc.NewServer()
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		panic(err)
	}
	testservice.RegisterTestServiceServer(srv, testservice.DefaultTestServiceServer)

	errs := make(chan error)

	go func() {
		log.Printf("listening on %s", lis.Addr().String())
		errs <- srv.Serve(lis)
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		log.Printf("shutdown due to %s", sig)
		srv.GracefulStop()
	}()

	if err := <-errs; err != nil {
		log.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func init() {
	flag.Parse()
}
