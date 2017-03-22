package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"crypto/tls"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/mwitkow/go-conntrack"
	"github.com/mwitkow/go-conntrack/connhelpers"
	"github.com/mwitkow/go-flagz"
	"github.com/mwitkow/go-grpc-middleware"
	"github.com/mwitkow/go-grpc-middleware/logging/logrus"
	"github.com/mwitkow/grpc-proxy/director"
	"github.com/mwitkow/grpc-proxy/proxy"
	"github.com/mwitkow/grpc-proxy/server/sharedflags"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	flagBindAddr         = sharedflags.Set.String("server_bind_address", "0.0.0.0", "address to bind the server to")
	flagGrpcInsecurePort = sharedflags.Set.Int("server_grpc_port", 8081, "TCP port to listen on for gRPC calls (insecure).")
	flagGrpcTlsPort      = sharedflags.Set.Int("server_grpc_tls_port", 8444, "TCP TLS port to listen on for secure gRPC calls. If 0, no gRPC-TLS will be open.")
	flagHttpPort         = sharedflags.Set.Int("server_http_port", 8080, "TCP port to listen on for HTTP1.1/REST calls (insecure, debug). If 0, no insecure HTTP will be open.")
	flagHttpTlsPort      = sharedflags.Set.Int("server_http_tls_port", 8443, "TCP port to listen on for HTTPS. If 0, no TLS will be open.")

	flagHttpMaxWriteTimeout = sharedflags.Set.Duration("server_http_max_write_timeout", 10*time.Second, "HTTP server config, max write duration.")
	flagHttpMaxReadTimeout  = sharedflags.Set.Duration("server_http_max_read_timeout", 10*time.Second, "HTTP server config, max read duration.")

	flagGrpcWebEnabled = sharedflags.Set.Bool("server_grpc_web_enabled", true, "Whether to enable gRPC-Web serving over HTTP ports.")

	flagGrpcWithTracing = sharedflags.Set.Bool("server_tracing_grpc_enabled", true, "Whether enable gRPC tracing (could be expensive).")
)

func main() {
	if err := sharedflags.Set.Parse(os.Args); err != nil {
		log.Fatalf("failed parsing flags: %v", err)
	}
	log.SetOutput(os.Stdout)
	grpc.EnableTracing = *flagGrpcWithTracing
	logEntry := log.NewEntry(log.StandardLogger())
	grpc_logrus.ReplaceGrpcLogger(logEntry)

	proxyDirector := director.New(buildBackendPoolOrFail(), buildRouterOrFail())
	grpcTlsCreds := newOptionalTlsCreds() // allows the server to listen both over tLS and nonTLS at the same time.
	grpcServer := grpc.NewServer(
		grpc.CustomCodec(proxy.Codec()), // needed for proxy to function.
		grpc.UnknownServiceHandler(proxy.TransparentHandler(proxyDirector)),
		grpc_middleware.WithUnaryServerChain(
			grpc_logrus.UnaryServerInterceptor(logEntry),
			grpc_prometheus.UnaryServerInterceptor,
		),
		grpc_middleware.WithStreamServerChain(
			grpc_logrus.StreamServerInterceptor(logEntry),
			grpc_prometheus.StreamServerInterceptor,
		),
		grpc.Creds(grpcTlsCreds),
	)
	//grpc_prometheus.Register(grpcServer)

	tlsConfig := buildServerTlsOrFail()

	httpServer := &http.Server{
		WriteTimeout: *flagHttpMaxWriteTimeout,
		ReadTimeout:  *flagHttpMaxReadTimeout,
		ErrorLog:     nil, // TODO(mwitkow): Add this to log to logrus.
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			log.Printf("got request: %v", req)
			if strings.HasPrefix(req.Header.Get("content-type"), "application/grpc") {
				if *flagGrpcWebEnabled {
					log.Printf("Serving grpc-web")
					grpcweb.WrapServer(grpcServer)(w, req)
					return
				} else {
					log.Printf("Serving grpc")
					grpcServer.ServeHTTP(w, req)
					return
				}
			}
			http.DefaultServeMux.ServeHTTP(w, req)
		}),
	}

	errChan := make(chan error)

	var grpcTlsListener net.Listener
	var grpcPlainListener net.Listener
	var httpPlainListener net.Listener
	var httpTlsListener net.Listener
	if *flagGrpcTlsPort != 0 {
		grpcTlsListener = buildListenerOrFail("grpc_tls", *flagGrpcTlsPort)
		grpcTlsCreds.addTlsListener(grpcTlsListener, credentials.NewTLS(tlsConfig))
	}
	if *flagGrpcInsecurePort != 0 {
		grpcPlainListener = buildListenerOrFail("grpc_plain", *flagGrpcInsecurePort)
	}
	if *flagHttpPort != 0 {
		httpPlainListener = buildListenerOrFail("http_plain", *flagHttpPort)
	}
	if *flagHttpTlsPort != 0 {
		httpTlsListener = buildListenerOrFail("http_tls", *flagHttpTlsPort)
		http2TlsConfig, err := connhelpers.TlsConfigWithHttp2Enabled(tlsConfig)
		if err != nil {
			log.Fatalf("failed setting up HTTP2 TLS config: %v", err)
		}
		httpTlsListener = tls.NewListener(httpTlsListener, http2TlsConfig)
	}

	if grpcTlsListener != nil {
		log.Infof("listening for gRPC TLS on: %v", grpcTlsListener.Addr().String())
		go func() {
			if err := grpcServer.Serve(grpcTlsListener); err != nil {
				errChan <- fmt.Errorf("grpc_tls server error: %v", err)
			}
		}()
	}
	if grpcPlainListener != nil {
		log.Infof("listening for gRPC Plain on: %v", grpcPlainListener.Addr().String())
		go func() {
			if err := grpcServer.Serve(grpcPlainListener); err != nil {
				errChan <- fmt.Errorf("grpc_plain server error: %v", err)
			}
		}()
	}
	if httpTlsListener != nil {
		log.Infof("listening for HTTP TLS on: %v", httpTlsListener.Addr().String())
		go func() {
			if err := httpServer.Serve(httpTlsListener); err != nil {
				errChan <- fmt.Errorf("http_tls server error: %v", err)
			}
		}()
	}
	if httpPlainListener != nil {
		log.Infof("listening for HTTP Plain on: %v", httpPlainListener.Addr().String())
		go func() {
			if err := httpServer.Serve(httpPlainListener); err != nil {
				errChan <- fmt.Errorf("http_plain server error: %v", err)
			}
		}()
	}
	err := <-errChan // this waits for some server breaking
	log.Fatalf("Error: %v", err)
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
