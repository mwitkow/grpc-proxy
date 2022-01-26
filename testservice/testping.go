package testservice

import (
	"context"
	"io"
	"reflect"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	returnHeader = "test-client-header"
)

// TestTestServiceServerImpl can be called to test the underlying TestServiceServer.
func TestTestServiceServerImpl(t *testing.T, client TestServiceClient) {
	t.Run("Unary ping", func(t *testing.T) {
		want := "hello, world"
		hdr := metadata.MD{}
		res, err := client.Ping(context.TODO(), &PingRequest{Value: want}, grpc.Header(&hdr))
		if err != nil {
			t.Errorf("want no err; got %v", err)
			return
		}
		checkHeaders(t, hdr)
		t.Logf("got %v (%d)", res.Value, res.Counter)
		if got := res.Value; got != want {
			t.Errorf("res.Value = %q; want %q", got, want)
		}
	})

	t.Run("Error ping", func(t *testing.T) {
		_, err := client.PingError(context.TODO(), &PingRequest{})
		if err == nil {
			t.Errorf("want err; got %v", err)
		}
	})

	t.Run("Server streaming ping", func(t *testing.T) {
		want := "hello, world"
		stream, err := client.PingList(context.TODO(), &PingRequest{Value: want})
		if err != nil {
			t.Errorf("want no err; got %v", err)
			if err := stream.CloseSend(); err != nil {
				t.Fatalf("closing send channel: %v", err)
			}
			return
		}
		hdr, err := stream.Header()
		if err != nil {
			t.Errorf("reading headers: %v", err)
		}
		checkHeaders(t, hdr)

		for {
			res, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					checkTrailers(t, stream.Trailer())
					return
				}
				t.Errorf("want no err; got %v", err)
				return
			}
			t.Logf("got %v (%d)", res.Value, res.Counter)
			if got := res.Value; got != want {
				t.Errorf("res.Value = %q; want %q", got, want)
			}
		}
	})

	t.Run("Bidirectional pinging", func(t *testing.T) {
		want := "hello, world"
		stream, err := client.PingStream(context.TODO())
		if err != nil {
			t.Errorf("want no err; got %v", err)
			if err := stream.CloseSend(); err != nil {
				t.Fatalf("closing send channel: %v", err)
			}
			return
		}

		d := make(chan struct{})
		go func() {
			hdr, err := stream.Header()
			if err != nil {
				t.Errorf("reading headers: %v", err)
			}
			checkHeaders(t, hdr)
			close(d)
		}()

		for i := 0; i < 25; i++ {
			if err := stream.Send(&PingRequest{Value: want}); err != nil {
				t.Errorf("want no err; got %v", err)
				return
			}
			res, err := stream.Recv()
			if err != nil {
				t.Errorf("receiving full duplex stream: %w", err)
				return
			}
			t.Logf("got %v (%d)", res.Value, res.Counter)
			if got := res.Value; got != want {
				t.Errorf("res.Value = %q; want %q", got, want)
			}
			if got, want := res.Counter, int32(i); got != want {
				t.Errorf("res.Counter = %d; want %d", got, want)
			}
		}
		if err := stream.CloseSend(); err != nil {
			t.Errorf("closing full duplex stream: %v", err)
		}
		<-d
	})

	t.Run("Unary ping with headers", func(t *testing.T) {
		want := "hello, world"
		req := &PingRequest{Value: want}

		ctx := metadata.AppendToOutgoingContext(context.Background(), returnHeader, "I like turtles.")
		inHeader := make(metadata.MD)

		res, err := client.Ping(ctx, req, grpc.Header(&inHeader))
		if err != nil {
			t.Errorf("want no err; got %v", err)
			return
		}
		t.Logf("got %v (%d)", res.Value, res.Counter)
		if !reflect.DeepEqual(inHeader.Get(returnHeader), []string{"I like turtles."}) {
			t.Errorf("did not receive correct return headers")
		}
	})
}

func checkTrailers(t *testing.T, md metadata.MD) {
	vs := md.Get(PingTrailer)
	if want, got := 1, len(vs); want != got {
		t.Errorf("trailer %q not present", PingTrailer)
		return
	}
	if want, got := []string{PingTrailerCts}, vs; !reflect.DeepEqual(got, want) {
		t.Errorf("trailer mismatch; want %q, got %q", want, got)
	}
}

func checkHeaders(t *testing.T, md metadata.MD) {
	vs := md.Get(PingHeader)
	if want, got := 1, len(vs); want != got {
		t.Errorf("header %q not present", PingHeader)
		return
	}
	if want, got := []string{PingHeaderCts}, vs; !reflect.DeepEqual(got, want) {
		t.Errorf("header mismatch; want %q, got %q", want, got)
	}
}
