package proxy_test

import (
	"context"
	"flag"
	"fmt"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/mwitkow/grpc-proxy/proxy"
	"github.com/mwitkow/grpc-proxy/testservice"
)

var testBackend = flag.String("test-backend", "", "Service providing TestServiceServer")

// TestIntegrationV1 is a regression test of the proxy.
func TestLegacyBehaviour(t *testing.T) {
	// These bufconns are test listeners used to make connections between our
	// services. This test actually starts two fully functional grpc services.
	proxyBc := bufconn.Listen(10)

	// Setup is a little thorough, but here's the gist of it:
	// 1. Create the test backend using testservice.DefaultTestServiceServer
	// 2. Create the proxy backend using this package
	// 3. Make calls to 1 via 2.

	// 1.
	//lint:ignore SA1019 regression test
	testCC, err := backendDialer(t, grpc.WithCodec(proxy.Codec()))
	if err != nil {
		t.Fatal(err)
	}

	// 2.
	go func() {
		// Second, we need to implement the SteamDirector.
		directorFn := func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			outCtx := metadata.NewOutgoingContext(ctx, md.Copy())
			return outCtx, testCC, nil
		}

		// Set up the proxy server and then serve from it like in step one.
		proxySrv := grpc.NewServer(
			//lint:ignore SA1019 regression test
			grpc.CustomCodec(proxy.Codec()), // was previously needed for proxy to function.
			grpc.UnknownServiceHandler(proxy.TransparentHandler(directorFn)),
		)
		// run the proxy backend
		go func() {
			t.Log("Running proxySrv")
			if err := proxySrv.Serve(proxyBc); err != nil {
				if err == grpc.ErrServerStopped {
					return
				}
				t.Logf("running proxy server: %v", err)
			}
		}()
		t.Cleanup(func() {
			t.Log("Gracefully stopping proxySrv")
			proxySrv.GracefulStop()
		})
	}()

	// 3.
	// Connect to the proxy. We should not need to do anything special here -
	// users do not need to know they're talking to a proxy.
	proxyCC, err := grpc.Dial(
		"bufnet",
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return proxyBc.Dial()
		}),
	)
	if err != nil {
		t.Fatalf("dialing proxy: %v", err)
	}
	proxyClient := testservice.NewTestServiceClient(proxyCC)

	// 4. Run the tests!
	testservice.TestTestServiceServerImpl(t, proxyClient)
}

func TestNewProxy(t *testing.T) {
	proxyBc := bufconn.Listen(10)

	// Setup is a little thorough, but here's the gist of it:
	// 1. Create the test backend using testservice.DefaultTestServiceServer
	// 2. Create the proxy backend using this package
	// 3. Make calls to 1 via 2.

	// 1.
	// First, we need to create a client connection to this backend.
	testCC, err := backendDialer(t)
	if err != nil {
		t.Fatal(err)
	}

	// 2.
	go func() {
		t.Helper()

		// First, we need to create a client connection to this backend.
		proxySrv := proxy.NewProxy(testCC)

		// run the proxy backend
		go func() {
			t.Log("Running proxySrv")
			if err := proxySrv.Serve(proxyBc); err != nil {
				if err == grpc.ErrServerStopped {
					return
				}
				t.Logf("running proxy server: %v", err)
			}
		}()
		t.Cleanup(func() {
			t.Log("Gracefully stopping proxySrv")
			proxySrv.GracefulStop()
		})
	}()

	// 3.
	// Connect to the proxy. We should not need to do anything special here -
	// users do not need to know they're talking to a proxy.
	t.Logf("dialing %s", proxyBc.Addr())
	proxyCC, err := grpc.Dial(
		proxyBc.Addr().String(),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return proxyBc.Dial()
		}),
	)
	if err != nil {
		t.Fatalf("dialing proxy: %v", err)
	}
	proxyClient := testservice.NewTestServiceClient(proxyCC)

	// 4. Run the tests!
	testservice.TestTestServiceServerImpl(t, proxyClient)
}

// backendDialer dials the testservice.TestServiceServer either by connecting
// to the user-supplied server, or by creating a mock server using bufconn.
func backendDialer(t *testing.T, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	t.Helper()

	if *testBackend != "" {
		return backendSvcDialer(t, *testBackend, opts...)
	}

	backendBc := bufconn.Listen(10)
	// set up the backend using a "real" server over a bufconn
	testSrv := grpc.NewServer()
	testservice.RegisterTestServiceServer(testSrv, testservice.DefaultTestServiceServer)

	// run the test backend
	go func() {
		t.Log("Running testSrv")
		if err := testSrv.Serve(backendBc); err != nil {
			if err == grpc.ErrServerStopped {
				return
			}
			t.Logf("running test server: %v", err)
		}
	}()
	t.Cleanup(func() {
		t.Log("Gracefully stopping testSrv")
		testSrv.GracefulStop()
	})

	opts = append(opts,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return backendBc.Dial()
		}),
	)

	backendCC, err := grpc.Dial(
		"bufnet",
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("dialing backend: %v", err)
	}
	return backendCC, nil
}

func backendSvcDialer(t *testing.T, addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = append(opts,
		grpc.WithInsecure(),
		grpc.WithBlock(),
	)

	t.Logf("connecting to %s", addr)
	cc, err := grpc.Dial(
		addr,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("dialing backend: %v", err)
	}

	return cc, nil
}
