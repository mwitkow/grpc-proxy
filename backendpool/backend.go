package backendpool

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/mwitkow/go-conntrack"
	"github.com/mwitkow/go-grpc-middleware"
	"github.com/mwitkow/go-srvlb/grpc"
	"github.com/mwitkow/go-srvlb/srv"
	pb "github.com/mwitkow/grpc-proxy/backendpool/proto"
	"github.com/mwitkow/grpc-proxy/proxy"
	"github.com/sercand/kuberesolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/naming"
)

var (
	ParentDialFunc = net.Dialer{
		Timeout:   1 * time.Second,
		KeepAlive: 30 * time.Second,
	}.DialContext
	ParentSrvResolver = srv.NewGoResolver(5 * time.Second)
)

type backend struct {
	conn   *grpc.ClientConn
	config *pb.Backend
}

func (b *backend) Conn() *grpc.ClientConn {
	return b.conn
}

func (b *backend) Close() error {
	return b.conn.Close()
}

func newBackend(cnf *pb.Backend) (*backend, error) {
	opts := []grpc.DialOption{}
	target, resolver, err := chooseNamingResolver(cnf)
	if err != nil {
		return nil, err
	}
	opts = append(opts, chooseDialFuncOpt(cnf))
	opts = append(opts, chooseSecurityOpt(cnf))
	opts = append(opts, grpc.WithCodec(proxy.Codec())) // needed for the proxy to function at all.
	opts = append(opts, chooseInterceptors(cnf)...)
	opts = append(opts, grpc.WithBalancer(chooseBalancerPolicy(cnf, resolver)))
	cc, err := grpc.Dial(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("backend '%v' dial error: %v", cnf.Name, err)
	}
	return &backend{conn: cc, config: cnf}, nil
}

func chooseDialFuncOpt(cnf *pb.Backend) grpc.DialOption {
	dialFunc := ParentDialFunc
	if !cnf.DisableConntracking {
		dialFunc = conntrack.NewDialContextFunc(
			conntrack.DialWithName(cnf.Name),
			conntrack.DialWithDialContextFunc(dialFunc),
		)
	}
	return grpc.WithDialer(func(addr string, t time.Duration) (net.Conn, error) {
		ctx, _ := context.WithTimeout(context.Background(), t)
		return dialFunc(ctx, "tcp", addr)
	})
}

func chooseSecurityOpt(cnf *pb.Backend) grpc.DialOption {
	if sec := cnf.GetSecurity(); sec != nil {
		config := &tls.Config{InsecureSkipVerify: true}
		if !sec.InsecureSkipVerify {
			// TODO(mwitkow): add configuration TlsConfig fetching by name here.
		}
		return grpc.WithTransportCredentials(credentials.NewTLS(config))
	} else {
		return grpc.WithInsecure()
	}
}

func chooseInterceptors(cnf *pb.Backend) []grpc.DialOption {
	unary := []grpc.UnaryClientInterceptor{}
	stream := []grpc.StreamClientInterceptor{}
	for _, i := range cnf.GetInterceptors() {
		if prom := i.GetPrometheus(); prom {
			unary = append(unary, grpc_prometheus.UnaryClientInterceptor)
			stream = append(stream, grpc_prometheus.StreamClientInterceptor)
		}
		// new interceptors are to be added here as else if statements.
	}
	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(unary...)),
		grpc.WithStreamInterceptor(grpc_middleware.ChainStreamClient(stream...)),
	}
}

func chooseNamingResolver(cnf *pb.Backend) (string, naming.Resolver, error) {
	if s := cnf.GetSrv(); s != nil {
		return s.GetDnsName(), grpcsrvlb.New(ParentSrvResolver), nil
	} else if k := cnf.GetK8S(); k != nil {
		// see https://github.com/sercand/kuberesolver/blob/master/README.md
		target := fmt.Sprintf("kubernetes://%v:%v", k.ServiceName, k.PortName)
		namespace := "default"
		if k.Namespace != "" {
			namespace = k.Namespace
		}
		b := kuberesolver.NewWithNamespace(namespace)
		return target, b.Resolver(), nil
	}
	return "", nil, fmt.Errorf("unspecified naming resolver for %v", cnf.Name)
}

func chooseBalancerPolicy(cnf *pb.Backend, resolver naming.Resolver) grpc.Balancer {
	switch cnf.GetBalancer() {
	case pb.Balancer_ROUND_ROBIN:
		return grpc.RoundRobin(resolver)
	default:
		return grpc.RoundRobin(resolver)
	}
}
