package proxy

import "google.golang.org/grpc"

type ErrorHandler func(conn *grpc.ClientConn)
