package main

import (
	"net"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/credentials"

	"github.com/Sirupsen/logrus"
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	pb_base "github.com/mwitkow/grpc-proxy/server/testclient/proto"

	"io"
	"os"

	"crypto/tls"

	"google.golang.org/grpc"
)

var (
	proxyHostPort = "127.0.0.1:8444" // use 8081 for plain text
)

func addClientCerts(tlsConfig *tls.Config) {
	cert, err := tls.LoadX509KeyPair("../misc/client.crt", "../misc/client.key")
	if err != nil {
		logrus.Fatal("failed loading client cert: %v", err)
	}
	tlsConfig.Certificates = []tls.Certificate{cert}
}

func main() {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // we use a self signed cert
	}
	addClientCerts(tlsConfig)
	logrus.SetOutput(os.Stdout)
	conn, err := grpc.Dial("controller.eu1-prod.improbable.local:9999",
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithDialer(spoofedGrpcDialer),
	)
	if err != nil {
		logrus.Fatalf("cannot dial: %v", err)
	}
	ctx, _ := context.WithTimeout(context.TODO(), 5*time.Second)
	client := pb_base.NewServerStatusClient(conn)
	listClient, err := client.FlagzList(ctx, &google_protobuf.Empty{})
	if err != nil {
		logrus.Fatalf("request failed: %v", err)
	}
	for {
		msg, err := listClient.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			logrus.Fatalf("request failed mid way: %v", err)
		}
		logrus.Info("Flag: ", msg)
	}
}

// spoofedGrpcDialer pretends to dial over a remote DNS name, but resolves to localhost.
// This is to send the requests to the proxy
func spoofedGrpcDialer(addr string, t time.Duration) (net.Conn, error) {
	host, _, _ := net.SplitHostPort(addr)
	switch host {
	case "controller.eu1-prod.improbable.local":
		return net.DialTimeout("tcp", proxyHostPort, t)
	default:
		return net.DialTimeout("tcp", addr, t)
	}
}
