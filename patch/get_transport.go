package grpc

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc/transport"
)

// GetTransport returns the balancer of the connection.
func (cc *ClientConn) GetTransport(ctx context.Context) (transport.ClientTransport, func(), error) {
	return cc.getTransport(ctx, BalancerGetOptions{})
}
