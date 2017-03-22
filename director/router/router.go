package router

import (
	pb "github.com/mwitkow/grpc-proxy/director/proto"

	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

var (
	emptyMd       = metadata.Pairs()
	routeNotFound = grpc.Errorf(codes.Unimplemented, "unknown route to service")
)

type Router interface {
	// Route returns a backend name for a given call, or an error.
	Route(ctx context.Context, fullMethodName string) (backendName string, err error)
}

type router struct {
	config *pb.Config
}

func NewStatic(cnf *pb.Config) *router {
	return &router{config: cnf}
}

func (r *router) Route(ctx context.Context, fullMethodName string) (backendName string, err error) {
	md, ok := metadata.FromContext(ctx)
	if !ok {
		md = emptyMd
	}
	for _, route := range r.config.Routes {
		if !r.serviceNameMatches(fullMethodName, route.ServiceNameMatcher) {
			continue
		}
		if !r.authorityMatches(md, route.AuthorityMatcher) {
			continue
		}
		if !r.metadataMatches(md, route.MetadataMatcher) {
			continue
		}
		return route.BackendName, nil
	}
	return "", routeNotFound
}

func (r *router) serviceNameMatches(fullMethodName string, matcher string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	if matcher[len(matcher)-1] == '*' {
		return strings.HasPrefix(fullMethodName, matcher[0:len(matcher)-1])
	}
	return fullMethodName == matcher
}

func (r *router) authorityMatches(md metadata.MD, matcher string) bool {
	if matcher == "" {
		return true
	}
	auth, ok := md[":authority"]
	if !ok || len(auth) == 0 {
		return false // there was no authority header and it was expected
	}
	return auth[0] == matcher
}

func (r *router) metadataMatches(md metadata.MD, expectedKv map[string]string) bool {
	for expK, expV := range expectedKv {
		vals, ok := md[strings.ToLower(expK)]
		if !ok {
			return false // key doesn't exist
		}
		found := false
		for _, v := range vals {
			if v == expV {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
