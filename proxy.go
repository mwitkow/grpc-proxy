package proxy

import (
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/transport"
)

var (
	clientStreamDescForProxying = &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}
)

func RegisterProxyStreams(server *grpc.Server, director StreamDirector, serviceName string, methodNames ...string) {
	streamer := &proxyStreamer{director}
	fakeDesc := &grpc.ServiceDesc{
		ServiceName: serviceName,
		HandlerType: (*interface{})(nil),
	}
	for _, m := range methodNames {
		streamDesc := grpc.StreamDesc{
			StreamName:    m,
			Handler:       streamer.handler,
			ServerStreams: true,
			ClientStreams: true,
		}
		fakeDesc.Streams = append(fakeDesc.Streams, streamDesc)
	}
	server.RegisterService(fakeDesc, streamer)
}

type proxyStreamer struct {
	director StreamDirector
}

// proxyStreamHandler is where the real magic of proxying happens.
// It is invoked like any gRPC server stream and uses the gRPC server framing to get and receive bytes from the wire,
// forwarding it to a ClientStream established against the relevant ClientConn.
func (s *proxyStreamer) handler(srv interface{}, serverStream grpc.ServerStream) error {
	backendConn, err := s.director(serverStream.Context())
	if err != nil {
		return err
	}
	// little bit of gRPC internals never hurt anyone
	lowLevelServerStream, ok := transport.StreamFromContext(serverStream.Context())
	if !ok {
		return grpc.Errorf(codes.Internal, "lowLevelServerStream not exists in context")
	}
	// TODO(mwitkow): Add a `forwarded` header to metadata, https://en.wikipedia.org/wiki/X-Forwarded-For.
	clientStream, err := grpc.NewClientStream(serverStream.Context(), clientStreamDescForProxying, backendConn, lowLevelServerStream.Method())
	if err != nil {
		return err
	}
	defer clientStream.CloseSend() // always close this!
	s2cErr := <-s.forwardServerToClient(serverStream, clientStream)
	c2sErr := <-s.forwardClientToServer(clientStream, serverStream)
	if s2cErr != io.EOF {
		return grpc.Errorf(codes.Internal, "failed proxying s2c: %v", s2cErr, c2sErr)
	}
	serverStream.SetTrailer(clientStream.Trailer())
	// c2sErr will contain RPC error from client code. If not io.EOF return the RPC error as server stream error.
	if c2sErr != io.EOF {
		return c2sErr
	}
	return nil
}

func (s *proxyStreamer) forwardClientToServer(src grpc.ClientStream, dst grpc.ServerStream) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &frame{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err // this can be io.EOF which is happy case
				break
			}
			if i == 0 {
				// This is a bit of a hack, but client to server headers are only readable after first client msg is
				// received but must be written to server stream before the first msg is flushed.
				// This is the only place to do it nicely.
				md, err := src.Header()
				if err != nil {
					ret <- err
					break
				}
				if err := dst.SendHeader(md); err != nil {
					ret <- err
					break
				}
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
		close(ret)
	}()
	return ret
}

func (s *proxyStreamer) forwardServerToClient(src grpc.ServerStream, dst grpc.ClientStream) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &frame{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err // this can be io.EOF which is happy case
				break
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
		close(ret)
	}()
	return ret
}
