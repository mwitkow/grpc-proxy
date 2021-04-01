package testservice

import (
	"context"
	"io"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var DefaultTestServiceServer = defaultPingServer{}

// defaultPingServer is the canonical implementation of a TestServiceServer.
type defaultPingServer struct {
	UnsafeTestServiceServer
}

func (s defaultPingServer) PingEmpty(ctx context.Context, empty *emptypb.Empty) (*PingResponse, error) {
	return &PingResponse{}, nil
}

func (s defaultPingServer) Ping(ctx context.Context, request *PingRequest) (*PingResponse, error) {
	headers, _ := metadata.FromIncomingContext(ctx)
	if h := headers.Get(returnHeader); len(h) > 0 {
		hdr := metadata.New(nil)
		hdr.Append(returnHeader, h...)
		if err := grpc.SendHeader(ctx, hdr); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to send headers: %v", err)
		}
	}
	return &PingResponse{Value: request.Value}, nil
}

func (s defaultPingServer) PingError(ctx context.Context, request *PingRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unknown, "Something is wrong and this is a message that describes it")
}

func (s defaultPingServer) PingList(request *PingRequest, server TestService_PingListServer) error {
	for i := 0; i < 10; i++ {
		if err := server.Send(&PingResponse{
			Value:   request.Value,
			Counter: int32(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s defaultPingServer) PingStream(server TestService_PingStreamServer) error {
	g, ctx := errgroup.WithContext(context.Background())
	pings := make(chan *PingRequest)
	g.Go(func() error {
		defer close(pings)
		for {
			m, err := server.Recv()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			select {
			case pings <- m:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
	g.Go(func() error {
		var i int32
		for m := range pings {
			if err := server.Send(&PingResponse{
				Value:   m.Value,
				Counter: i,
			}); err != nil {
				return err
			}
			i++
		}
		return nil
	})

	return g.Wait()
}

var _ TestServiceServer = (*defaultPingServer)(nil)
