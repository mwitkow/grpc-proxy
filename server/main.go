package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/mwitkow/bazel-distcache/proto/build/remote"
	"github.com/mwitkow/bazel-distcache/service/cas"
	"github.com/mwitkow/bazel-distcache/service/executioncache"
	"github.com/mwitkow/go-conntrack"
	"github.com/mwitkow/go-flagz"
	"github.com/mwitkow/go-grpc-prometheus"
	"github.com/mwitkow/grpc-proxy/server/sharedflags"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

var (
	flagBindAddr      = sharedflags.Set.String("server_bind_address", "0.0.0.0", "address to bind the server to")
	flagTlsServerCert = sharedflags.Set.String(
		"server_tls_cert_file",
		"misc/localhost.pem",
		"Path to the PEM certificate for server use.")
	flagTlsServerKey = sharedflags.Set.String(
		"server_tls_key_file",
		"misc/localhost.key",
		"Path to the PEM key for the certificate for the server use.")

	flagGrpcInsecurePort = sharedflags.Set.Int("server_grpc_port", 0, "TCP port to listen on for gRPC calls (insecure).")
	flagGrpcTlsPort      = sharedflags.Set.Int("server_grpc_tls_port", 0, "TCP TLS port to listen on for secure gRPC calls. If 0, no gRPC-TLS will be open.")
	flagHttpPort         = sharedflags.Set.Int("server_http_port", 0, "TCP port to listen on for HTTP1.1/REST calls (insecure, debug). If 0, no insecure HTTP will be open.")
	flagHttpTlsPort      = sharedflags.Set.Int("server_http_tls_port", 0, "TCP port to listen on for HTTPS. If 0, no TLS will be open.")

	flagGrpcWebEnabled = sharedflags.Set.Bool("server_grpc_web_enabled", true, "Whether to enable gRPC-Web serving over HTTP ports.")

	flagGrpcWithTracing = sharedflags.Set.Bool("server_tracing_grpc_enabled", true, "Whether enable gRPC tracing (could be expensive).")
)

func main() {

	if err := sharedflags.Set.Parse(os.Args); err != nil {
		log.Fatalf("failed parsing flags: %v", err)
	}
	grpc.EnableTracing = *flagGrpcWithTracing

	// TODO(mwitkow): Sort it out.
	grpclog.SetLogger(log.StandardLogger())
	grpcServer := grpc.NewServer(
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)

	grpc_prometheus.Register(grpcServer)

	go func() {
		log.Infof("listening for HTTP (debug) on: http://%v", httpListener.Addr().String())
		http.Serve(httpListener, http.DefaultServeMux)
	}()

	log.Infof("listening for gRPC (bazel) on: %v", grpcListener.Addr().String())
	if err := grpcServer.Serve(grpcListener); err != nil {
		log.Fatalf("failed staring gRPC server: %v", err)
	}
}

func registerDebugHandlers() {
	// TODO(mwitkow): Add middleware for making these only visible to private IPs.
	http.Handle("/debug/metrics", prometheus.UninstrumentedHandler())
	http.Handle("/debug/flagz", http.HandlerFunc(flagz.NewStatusEndpoint(sharedflags.Set).ListFlags))
	http.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	http.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	http.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	http.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	http.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	http.Handle("/debug/events", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		trace.Render(w, req /*sensitive*/, true)
	}))
	http.Handle("/debug/events", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		trace.RenderEvents(w, req /*sensitive*/, true)
	}))
}

func buildListenerOrFail(name string, port int) net.Listener {
	addr := fmt.Sprintf("%s:%d", *flagBindAddr, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed listening for '%v' on %v: %v", name, port, err)
	}
	return conntrack.NewListener(listener,
		conntrack.TrackWithName(name),
		conntrack.TrackWithTcpKeepAlive(20*time.Second),
		conntrack.TrackWithTracing(),
	)
}
