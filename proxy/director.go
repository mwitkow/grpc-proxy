// Copyright 2017 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package proxy

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// StreamDirector manages gRPC Client connections used to forward requests.
//
// The presence of the `Context` allows for rich filtering, e.g. based on
// Metadata (headers). If no handling is meant to be done, a
// `codes.NotImplemented` gRPC error should be returned.
//
// It is worth noting that the Connect will be called *after* all server-side
// stream interceptors are invoked. So decisions around authorization,
// monitoring etc. are better to be handled there.
type StreamDirector interface {
	// Connect returns a connection to use for the given method,
	// or an error if the call should not be handled.
	//
	// The provided context may be inspected for filtering on request
	// metadata.
	//
	// The returned context is used as the basis for the outgoing connection.
	Connect(ctx context.Context, method string) (context.Context, *grpc.ClientConn, error)

	// Release is called when a connection is longer being used.  This is called
	// once for every call to Connect that does not return an error.
	//
	// The provided context is the one returned from Connect.
	//
	// This can be used by the director to pool connections or close unused
	// connections.
	Release(ctx context.Context, conn *grpc.ClientConn)
}
