package backendpool

import (
	"fmt"

	pb "github.com/mwitkow/grpc-proxy/backendpool/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	ErrUnknownBackend = grpc.Errorf(codes.Unimplemented, "unknown backend")
)

type Pool interface {
	// Conn returns a dialled grpc.ClientConn for a given backend name.
	Conn(backendName string) (*grpc.ClientConn, error)
}

// static is a Pool with a static configuration.
type static struct {
	backends map[string]*backend
}

// NewStatic creates a backend pool that has static configuration.
func NewStatic(config *pb.Config) (Pool, error) {
	s := &static{backends: make(map[string]*backend)}
	for _, beCnf := range config.Backends {
		be, err := newBackend(beCnf)
		if err != nil {
			return nil, fmt.Errorf("failed creating backend '%v': %v", beCnf.Name, err)
		}
		s.backends[beCnf.Name] = be
	}
	return s, nil
}

func (s *static) Conn(backendName string) (*grpc.ClientConn, error) {
	be, ok := s.backends[backendName]
	if !ok {
		return nil, ErrUnknownBackend
	}
	return be.Conn(), nil
}
