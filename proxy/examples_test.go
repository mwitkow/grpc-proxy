// Copyright 2017 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package proxy

import (
	"context"
	"log"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

var (
	director StreamDirector
)

func ExampleNewProxy() {
	dst, err := grpc.Dial("example.com")
	if err != nil {
		log.Fatalf("dialing example.org: %v", err)
	}
	proxy := NewProxy(dst)
	_ = proxy
}

func ExampleRegisterService() {
	// A gRPC server with the proxying codec enabled.
	server := grpc.NewServer()
	// Register a TestService with 4 of its methods explicitly.
	RegisterService(server, director,
		"mwitkow.testproto.TestService",
		"PingEmpty", "Ping", "PingError", "PingList")
}

func ExampleTransparentHandler() {
	grpc.NewServer(
		grpc.UnknownServiceHandler(TransparentHandler(director)))
}

// Provides a simple example of a director that shields internal services and dials a staging or production backend.
// This is a *very naive* implementation that creates a new connection on every request. Consider using pooling.
func ExampleStreamDirector() {
	director = func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
		// Make sure we never forward internal services.
		if strings.HasPrefix(fullMethodName, "/com.example.internal.") {
			return nil, nil, grpc.Errorf(codes.Unimplemented, "Unknown method")
		}
		md, ok := metadata.FromIncomingContext(ctx)
		// Copy the inbound metadata explicitly.
		outCtx := metadata.NewOutgoingContext(ctx, md.Copy())
		if ok {
			// Decide on which backend to dial
			if val, exists := md[":authority"]; exists && val[0] == "staging.api.example.com" {
				// Make sure we use DialContext so the dialing can be cancelled/time out together with the context.
				conn, err := grpc.DialContext(ctx, "api-service.staging.svc.local", grpc.WithCodec(Codec()))
				return outCtx, conn, err
			} else if val, exists := md[":authority"]; exists && val[0] == "api.example.com" {
				conn, err := grpc.DialContext(ctx, "api-service.prod.svc.local", grpc.WithCodec(Codec()))
				return outCtx, conn, err
			}
		}
		return nil, nil, grpc.Errorf(codes.Unimplemented, "Unknown method")
	}
}
