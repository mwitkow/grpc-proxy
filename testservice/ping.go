package testservice

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var DefaultTestServiceServer = defaultPingServer{}

const (
	PingHeader      = "ping-header"
	PingHeaderCts   = "Arbitrary header text"
	PingTrailer     = "ping-trailer"
	PingTrailerCts  = "Arbitrary trailer text"
	PingEchoHeader  = "ping-echo-header"
	PingEchoTrailer = "ping-echo-trailer"
)

// defaultPingServer is the canonical implementation of a TestServiceServer.
type defaultPingServer struct {
	UnsafeTestServiceServer
}

func (s defaultPingServer) PingEmpty(ctx context.Context, empty *emptypb.Empty) (*PingResponse, error) {
	if err := s.sendHeader(ctx); err != nil {
		return nil, err
	}
	if err := s.setTrailer(ctx); err != nil {
		return nil, err
	}
	return &PingResponse{}, nil
}

func (s defaultPingServer) Ping(ctx context.Context, request *PingRequest) (*PingResponse, error) {
	if err := s.sendHeader(ctx); err != nil {
		return nil, err
	}
	if err := s.setTrailer(ctx); err != nil {
		return nil, err
	}

	return &PingResponse{Value: request.Value}, nil
}

func (s defaultPingServer) PingError(ctx context.Context, request *PingRequest) (*emptypb.Empty, error) {
	if err := s.sendHeader(ctx); err != nil {
		return nil, err
	}
	if err := s.setTrailer(ctx); err != nil {
		return nil, err
	}
	return nil, status.Error(codes.Unknown, "Something is wrong and this is a message that describes it")
}

func (s defaultPingServer) PingList(request *PingRequest, server TestService_PingListServer) error {
	if err := s.sendHeader(server.Context()); err != nil {
		return err
	}
	s.setStreamTrailer(server)
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

	if err := s.sendHeader(server.Context()); err != nil {
		return err
	}

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

func (s *defaultPingServer) sendHeader(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	if tvs := md.Get(PingEchoHeader); len(tvs) > 0 {
		md.Append(PingEchoHeader, tvs...)
	}

	md.Append(PingHeader, PingHeaderCts)

	if err := grpc.SendHeader(ctx, md); err != nil {
		return fmt.Errorf("setting header: %w", err)
	}
	return nil
}

func (s *defaultPingServer) setTrailer(ctx context.Context) error {
	md := s.buildTrailer(ctx)

	if err := grpc.SetTrailer(ctx, md); err != nil {
		return fmt.Errorf("setting trailer: %w", err)
	}

	return nil
}

func (s *defaultPingServer) buildTrailer(ctx context.Context) metadata.MD {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	if tvs := md.Get(PingEchoTrailer); len(tvs) > 0 {
		md.Append(PingEchoTrailer, tvs...)
	}

	md.Append(PingTrailer, PingTrailerCts)

	return md
}

func (s defaultPingServer) setStreamTrailer(server grpc.ServerStream) {
	server.SetTrailer(s.buildTrailer(server.Context()))
}

var _ TestServiceServer = (*defaultPingServer)(nil)
