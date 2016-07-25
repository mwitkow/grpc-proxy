// Copyright © 2015 Michal Witkowski <michal@improbable.io>
// Copyright 2014, Google Inc. - server parts, licensed under MIT license.
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

package proxy

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/transport"
)

// transportWriter is a common interface between gRPC transport.ServerTransport and transport.ClientTransport.
type transportWriter interface {
	Write(s *transport.Stream, data []byte, opts *transport.Options) error
}

type Proxy struct {
	mu       sync.Mutex
	lis      map[net.Listener]bool
	conns    map[transport.ServerTransport]bool
	logger   grpclog.Logger
	director StreamDirector
	opts     *options
}

// NewServer creates a gRPC proxy which will use the `StreamDirector` for making routing decisions.
func NewServer(director StreamDirector, opt ...ProxyOption) *Proxy {
	s := &Proxy{
		lis:      make(map[net.Listener]bool),
		conns:    make(map[transport.ServerTransport]bool),
		opts:     &options{},
		director: director,
		logger:   &defaultLogger{},
	}
	for _, o := range opt {
		o(s.opts)
	}
	if s.opts.logger != nil {
		s.logger = s.opts.logger
	}
	return s
}

// Serve handles the serving path of the grpc.
func (s *Proxy) Serve(lis net.Listener) error {
	s.mu.Lock()
	if s.lis == nil {
		s.mu.Unlock()
		return grpc.ErrServerStopped
	}
	s.lis[lis] = true
	s.mu.Unlock()
	defer func() {
		lis.Close()
		s.mu.Lock()
		delete(s.lis, lis)
		s.mu.Unlock()
	}()
	for {
		c, err := lis.Accept()
		if err != nil {
			s.mu.Lock()
			s.mu.Unlock()
			return err
		}
		var authInfo credentials.AuthInfo = nil
		if creds, ok := s.opts.creds.(credentials.TransportCredentials); ok {
			var conn net.Conn
			conn, authInfo, err = creds.ServerHandshake(c)
			if err != nil {
				s.mu.Lock()
				s.mu.Unlock()
				s.logger.Println("grpc: Proxy.Serve failed to complete security handshake.")
				continue
			}
			c = conn
		}
		s.mu.Lock()
		if s.conns == nil {
			s.mu.Unlock()
			c.Close()
			return nil
		}
		st, err := transport.NewServerTransport("http2", c, s.opts.maxConcurrentStreams, authInfo)
		if err != nil {
			s.mu.Unlock()
			c.Close()
			s.logger.Println("grpc: Proxy.Serve failed to create ServerTransport: ", err)
			continue
		}
		s.conns[st] = true
		s.mu.Unlock()

		var wg sync.WaitGroup
		st.HandleStreams(func(stream *transport.Stream) {
			wg.Add(1)
			go func() {
				s.handleStream(st, stream)
				wg.Done()
			}()
		})
		wg.Wait()
		s.mu.Lock()
		delete(s.conns, st)
		s.mu.Unlock()
	}
}

func (s *Proxy) handleStream(frontTrans transport.ServerTransport, frontStream *transport.Stream) {
	sm := frontStream.Method()
	if sm != "" && sm[0] == '/' {
		sm = sm[1:]
	}
	pos := strings.LastIndex(sm, "/")
	if pos == -1 {
		if err := frontTrans.WriteStatus(frontStream, codes.InvalidArgument, fmt.Sprintf("malformed method name: %q", frontStream.Method())); err != nil {
			s.logger.Printf("proxy: Proxy.handleStream failed to write status: %v", err)
		}
		return
	}
	ProxyStream(s.director, s.logger, frontTrans, frontStream)

}

// Stop stops the gRPC server. Once Stop returns, the server stops accepting
// connection requests and closes all the connected connections.
func (s *Proxy) Stop() {
	s.mu.Lock()
	listeners := s.lis
	s.lis = nil
	cs := s.conns
	s.conns = nil
	s.mu.Unlock()
	for lis := range listeners {
		lis.Close()
	}
	for c := range cs {
		c.Close()
	}
}

// ProxyStream performs a forward of a gRPC frontend stream to a backend.
func ProxyStream(director StreamDirector, logger grpclog.Logger, frontTrans transport.ServerTransport, frontStream *transport.Stream) {
	backendTrans, backendStream, err := backendTransportStream(director, frontStream.Context())
	if err != nil {
		frontTrans.WriteStatus(frontStream, grpc.Code(err), grpc.ErrorDesc(err))
		logger.Printf("proxy: Proxy.handleStream %v failed to allocate backend: %v", frontStream.Method(), err)
		return
	}
	defer backendTrans.CloseStream(backendStream, nil)

	// data coming from client call to backend
	ingressPathChan := forwardDataFrames(frontStream, backendStream, backendTrans)

	// custom header handling *must* be after some data is processed by the backend, otherwise there's a deadlock
	headerMd, err := backendStream.Header()
	if err == nil && len(headerMd) > 0 {
		frontTrans.WriteHeader(frontStream, headerMd)
	}
	// data coming from backend back to client call
	egressPathChan := forwardDataFrames(backendStream, frontStream, frontTrans)

	// wait for both data streams to complete.
	egressErr := <-egressPathChan
	ingressErr := <-ingressPathChan
	if egressErr != io.EOF || ingressErr != io.EOF {
		logger.Printf("proxy: Proxy.handleStream %v failure during transfer ingres: %v egress: %v", frontStream.Method(), ingressErr, egressErr)
		frontTrans.WriteStatus(frontStream, codes.Unavailable, fmt.Sprintf("problem in transfer ingress: %v egress: %v", ingressErr, egressErr))
		return
	}
	// handle trailing metadata
	trailingMd := backendStream.Trailer()
	if len(trailingMd) > 0 {
		frontStream.SetTrailer(trailingMd)
	}
	frontTrans.WriteStatus(frontStream, backendStream.StatusCode(), backendStream.StatusDesc())
}

// backendTransportStream picks and establishes a Stream to the backend.
func backendTransportStream(director StreamDirector, ctx context.Context) (transport.ClientTransport, *transport.Stream, error) {
	grpcConn, err := director(ctx)
	if err != nil {
		if grpc.Code(err) != codes.Unknown { // rpcError check
			return nil, nil, err
		} else {
			return nil, nil, grpc.Errorf(codes.Aborted, "cant dial to backend: %v", err)
		}
	}
	// TODO(michal): ClientConn.GetTransport() IS NOT IN UPSTREAM GRPC!
  // To make this work, copy patch/get_transport.go to google.golang.org/grpc/
	backendTrans, _, err := grpcConn.GetTransport(ctx)
	frontendStream, _ := transport.StreamFromContext(ctx)
	callHdr := &transport.CallHdr{
		Method: frontendStream.Method(),
		Host:   "TODOFIXTLS", // TODO(michal): This can fail if the backend server is using TLS Hostname verification. Use conn.authority, once it's public?
	}
	backendStream, err := backendTrans.NewStream(ctx, callHdr)
	if err != nil {
		return nil, nil, grpc.Errorf(codes.Unknown, "cant establish stream to backend: %v", err)
	}
	return backendTrans, backendStream, nil
}

// forwardDataFrames moves data from one gRPC transport `Stream` to another in async fashion.
// It returns an error channel. `nil` on it signifies everything was fine, anything else is a serious problem.
func forwardDataFrames(srcStream *transport.Stream, dstStream *transport.Stream, dstTransport transportWriter) chan error {
	ret := make(chan error, 1)

	go func() {
		data := make([]byte, 4096)
		opt := &transport.Options{}
		for {
			n, err := srcStream.Read(data)
			if err != nil {  // including io.EOF
				// Send nil to terminate the stream.
				opt.Last = true
				dstTransport.Write(dstStream, nil, opt)
				ret <- err
				break
			}
			if err := dstTransport.Write(dstStream, data[:n], opt); err != nil {
				ret <- err
				break
			}
		}
		close(ret)
	}()
	return ret
}

