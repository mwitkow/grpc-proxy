// Copyright © 2015 Michal Witkowski <michal@improbable.io>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy_test
import (
	"strings"
	"time"
	"net"
	"testing"
	"io"


	"github.com/mwitkow/grpc-proxy"
	pb "github.com/mwitkow/grpc-proxy/testservice"

	"github.com/stretchr/testify/suite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"golang.org/x/net/context"
	"github.com/stretchr/testify/assert"
)


const (
	pingDefaultValue = "I like kittens."
	clientMdKey = "test-client-header"
	serverHeaderMdKey = "test-client-header"
	serverTrailerMdKey = "test-client-trailer"

	rejectingMdKey = "test-reject-rpc-if-in-context"

	countListResponses = 20
)

// asserting service is implemented on the server side and serves as a handler for stuff
type assertingService struct {
	logger *grpclog.Logger
	t      *testing.T
}

func (s *assertingService) PingEmpty(ctx context.Context, _ *pb.Empty) (*pb.PingResponse, error) {
	// Check that this call has client's metadata.
	md, ok := metadata.FromContext(ctx)
	assert.True(s.t, ok, "PingEmpty call must have metadata in context")
	_, ok = md[clientMdKey]
	assert.True(s.t, ok, "PingEmpty call must have clients's custom headers in metadata")
	return &pb.PingResponse{Value: pingDefaultValue, Counter: 42}, nil
}

func (s *assertingService) Ping(ctx context.Context, ping *pb.PingRequest) (*pb.PingResponse, error) {
	// Send user trailers and headers.
	grpc.SendHeader(ctx, metadata.Pairs(serverHeaderMdKey, "I like turtles."))
	grpc.SetTrailer(ctx, metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return &pb.PingResponse{Value: ping.Value, Counter: 42}, nil
}

func (s *assertingService) PingError(ctx context.Context, ping *pb.PingRequest) (*pb.Empty, error) {
	return nil, grpc.Errorf(codes.FailedPrecondition, "Userspace error.")
}

func (s *assertingService) PingList(ping *pb.PingRequest, stream pb.TestService_PingListServer) error {
	// Send user trailers and headers.
	stream.SendHeader(metadata.Pairs(serverHeaderMdKey, "I like turtles."))
	for i := 0; i < countListResponses; i++ {
		stream.Send(&pb.PingResponse{Value: ping.Value, Counter: int32(i)})
	}
	stream.SetTrailer(metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return nil
}


// ProxyHappySuite tests the "happy" path of handling: that everything works in absence of connection issues.
type ProxyHappySuite struct {
	suite.Suite

	serverListener net.Listener
	server         *grpc.Server
	proxyListener  net.Listener
	proxy          *proxy.Proxy

	client         *grpc.ClientConn
	testClient     pb.TestServiceClient

	ctx            context.Context
}

func (s *ProxyHappySuite) TestPingEmptyCarriesClientMetadata() {
	ctx := metadata.NewContext(s.ctx, metadata.Pairs(clientMdKey, "true"))
	out, err := s.testClient.PingEmpty(ctx, &pb.Empty{})
	require.NoError(s.T(), err, "PingEmpty should succeed without errors")
	require.Equal(s.T(), &pb.PingResponse{Value: pingDefaultValue, Counter: 42}, out)
}

func (s *ProxyHappySuite) TestPingCarriesServerHeadersAndTrailers() {
	headerMd := make(metadata.MD)
	trailerMd := make(metadata.MD)
	// This is an awkward calling convention... but meh.
	out, err := s.testClient.Ping(s.ctx, &pb.PingRequest{Value: "foo"}, grpc.Header(&headerMd), grpc.Trailer(&trailerMd))
	require.NoError(s.T(), err, "Ping should succeed without errors")
	require.Equal(s.T(), &pb.PingResponse{Value: "foo", Counter: 42}, out)
	assert.Len(s.T(), headerMd, 1, "server response headers must contain server data")
	assert.Len(s.T(), trailerMd, 1, "server response trailers must contain server data")
}

func (s *ProxyHappySuite) TestPingErrorPropagatesAppError() {
	_, err := s.testClient.PingError(s.ctx, &pb.PingRequest{Value: "foo"})
	require.Error(s.T(), err, "PingError should never succeed")
	assert.Equal(s.T(), codes.FailedPrecondition, grpc.Code(err))
	assert.Equal(s.T(), "Userspace error.", grpc.ErrorDesc(err))
}

func (s *ProxyHappySuite) TestDirectorErrorIsPropagated() {
	// See SetupSuite where the StreamDirector has a special case.
	ctx := metadata.NewContext(s.ctx, metadata.Pairs(rejectingMdKey, "true"))
	_, err := s.testClient.Ping(ctx, &pb.PingRequest{Value: "foo"})
	require.Error(s.T(), err, "Director should reject this RPC")
	assert.Equal(s.T(), codes.PermissionDenied, grpc.Code(err))
	assert.Equal(s.T(), "testing rejection", grpc.ErrorDesc(err))
}

func (s *ProxyHappySuite) TestPingListStreamsAll() {
	stream, err := s.testClient.PingList(s.ctx, &pb.PingRequest{Value: "foo"})
	require.NoError(s.T(), err, "PingList request should be successful.")
	// Check that the header arrives before all entries.
	headerMd, err := stream.Header()
	require.NoError(s.T(), err, "PingList headers should not error.")
	assert.Len(s.T(), headerMd, 1, "PingList response headers user contain metadata")
	count := 0
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(s.T(), err, "PingList stream should not be interrupted.")
		require.Equal(s.T(), "foo", resp.Value)
		count = count + 1
	}
	assert.Equal(s.T(), countListResponses, count, "PingList must successfully return all outputs")
	// Check that the trailer headers are here.
	trailerMd := stream.Trailer()
	assert.Len(s.T(), trailerMd, 1, "PingList trailer headers user contain metadata")
}

func (s *ProxyHappySuite) SetupSuite() {
	var err error
	logger := &testingLog{(*s.T())}

	s.proxyListener, err = net.Listen("tcp", "127.0.0.1:0")
	require.NoError(s.T(), err, "must be able to allocate a port for proxyListener")
	s.serverListener, err = net.Listen("tcp", "127.0.0.1:0")
	require.NoError(s.T(), err, "must be able to allocate a port for serverListener")

	s.server = grpc.NewServer()
	pb.RegisterTestServiceServer(s.server, &assertingService{t: s.T()})

	// Setup of the proxy's Director.
	proxyClientConn, err := grpc.Dial(s.serverListener.Addr().String(), grpc.WithInsecure())
	require.NoError(s.T(), err, "must not error on deferred client Dial")
	proxyServer := proxy.NewServer(func(ctx context.Context) (*grpc.ClientConn, error) {
		md, ok := metadata.FromContext(ctx)
		if ok {
			if _, exists := md[rejectingMdKey]; exists {
				return nil, grpc.Errorf(codes.PermissionDenied, "testing rejection")
			}
		}
		return proxyClientConn, nil
	}, proxy.UsingLogger(logger))

	// Start the serving loops.
	go func() {
		s.T().Logf("starting grpc.Server at: %v", s.serverListener.Addr().String())
		s.server.Serve(s.serverListener)
	}()
	go func() {
		s.T().Logf("starting grpc.Proxy at: %v", s.proxyListener.Addr().String())
		proxyServer.Serve(s.proxyListener)
	}()

	clientConn, err := grpc.Dial(strings.Replace(s.proxyListener.Addr().String(), "127.0.0.1", "localhost", 1), grpc.WithInsecure())
	require.NoError(s.T(), err, "must not error on deferred client Dial")
	s.testClient = pb.NewTestServiceClient(clientConn)
	// Make all RPC calls last at most 1 sec, meaning all async issues or deadlock will not kill tests.
	s.ctx, _ = context.WithTimeout(context.TODO(), 1 * time.Second)
}

func (s *ProxyHappySuite) TearDownSuite() {
	if s.proxy != nil {
		s.proxy.Stop()
		s.proxyListener.Close()
	}
	if s.serverListener != nil {
		s.server.Stop()
		s.serverListener.Close()
	}
	if s.client != nil {
		s.client.Close()
	}
}

func TestProxyHappySuite(t *testing.T) {
	suite.Run(t, &ProxyHappySuite{})
}

// Abstraction that allows us to pass the *testing.T as a grpclogger.
type testingLog struct {
	testing.T
}

func (t *testingLog) Fatalln(args ...interface{}) {
	t.T.Fatal(args...)
}

func (t *testingLog) Print(args ...interface{}) {
	t.T.Log(args...)
}

func (t *testingLog) Printf(format string, args ...interface{}) {
	t.T.Logf(format, args...)
}


func (t *testingLog) Println(args ...interface{}) {
	t.T.Log(args...)
}
