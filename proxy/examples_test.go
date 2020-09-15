// Copyright 2017 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package proxy_test

import (
	"strings"
	"time"

	"github.com/mwitkow/grpc-proxy/proxy"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

var (
	exampleDirector proxy.StreamDirector
)

func ExampleRegisterService() {
	// init grpc conn pool
	examplePool = proxy.NewPool(300, time.Duration(60000)*time.Millisecond, 500, 500)
	// A gRPC server with the proxying codec enabled.
	encoding.RegisterCodec(proxy.Codec())
	server := grpc.NewServer()
	// Register a TestService with 4 of its methods explicitly.
	proxy.RegisterService(server, exampleDirector,
		"mwitkow.testproto.TestService",
		"PingEmpty", "Ping", "PingError", "PingList")
}

func ExampleTransparentHandler() {
	examplePool = proxy.NewPool(300, time.Duration(60000)*time.Millisecond, 500, 500)
	encoding.RegisterCodec(proxy.Codec())
	grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentHandler(exampleDirector)))
}

// Provide sa simple example of a director that shields internal services and dials a staging or production backend.
// This is a *very naive* implementation that creates a new connection on every request. Consider using pooling.
func ExampleStreamDirector() {
	exampleDirector = func(ctx context.Context, fullMethodName string) (context.Context, *proxy.PoolConn, error) {
		// Make sure we never forward internal services.
		if strings.HasPrefix(fullMethodName, "/com.example.internal.") {
			return nil, nil, status.Errorf(codes.Unimplemented, "Unknown method")
		}
		md, ok := metadata.FromIncomingContext(ctx)
		// Copy the inbound metadata explicitly.
		outCtx, _ := context.WithCancel(ctx)
		outCtx = metadata.NewOutgoingContext(outCtx, md.Copy())
		if ok {
			// Decide on which backend to dial
			if val, exists := md[":authority"]; exists && val[0] == "staging.api.example.com" {
				// Make sure we use DialContext so the dialing can be cancelled/time out together with the context.
				backendConn, err := examplePool.GetConn("api-service.staging.svc.local", grpc.WithDefaultCallOptions(grpc.ForceCodec(proxy.Codec())),
					grpc.WithInsecure())
				return outCtx, backendConn, err
			} else if val, exists := md[":authority"]; exists && val[0] == "api.example.com" {
				backendConn, err := examplePool.GetConn("api-service.prod.svc.local", grpc.WithDefaultCallOptions(grpc.ForceCodec(proxy.Codec())),
					grpc.WithInsecure())
				return outCtx, backendConn, err
			}
		}
		return nil, nil, status.Errorf(codes.Unimplemented, "Unknown method")
	}
}
