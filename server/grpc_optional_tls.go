package main

import (
	"net"
	"google.golang.org/grpc/credentials"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// optionalTlsCreds acts as gRPC `credentials.TransportAuthenticator` but checks if the connection came from an TLS-configured listener.
type optionalTlsCreds struct {
	tlsForPort map[string]credentials.TransportCredentials
}

// optionalTlsCreds acts as gRPC `credentials.TransportAuthenticator` but checks if the connection came from an TLS-configured listener.
func newOptionalTlsCreds() *optionalTlsCreds {
	return &optionalTlsCreds{
		tlsForPort: make(map[string]credentials.TransportCredentials),
	}
}

func (c *optionalTlsCreds) addTlsListener(listener net.Listener, creds credentials.TransportCredentials) {
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	c.tlsForPort[port] = creds
}

// Only needed to implement the interface, testclient side functionality, unused on server.
func (c *optionalTlsCreds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{
		SecurityProtocol: "tls",
		SecurityVersion:  "1.2",
	}
}

// Only needed to implement the interface, testclient side functionality, unused on server.
func (c *optionalTlsCreds) ClientHandshake(ctx context.Context, addr string, rawConn net.Conn) (_ net.Conn, _ credentials.AuthInfo, err error) {
	return nil, nil, grpc.Errorf(codes.Unimplemented, "You can't use optionalTlsCreds for testclient connecitons.")
}

func (c *optionalTlsCreds) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	_, port, err := net.SplitHostPort(rawConn.LocalAddr().String())
	if err != nil {
		return nil, nil, grpc.Errorf(codes.Unimplemented, "Cant resolve port in optionalTlsCreds")
	}
	if tlsCreds, ok := c.tlsForPort[port]; ok {
		conn, info, err := tlsCreds.ServerHandshake(rawConn)
		if err != nil {
			return nil, nil, err
		}
		return conn, info, nil
	}
	// non-TLS case.
	return rawConn, nil, nil
}

func (c *optionalTlsCreds) OverrideServerName(serverNameOverride string) error {
	for _, v := range c.tlsForPort {
		if err := v.OverrideServerName(serverNameOverride); err != nil {
			return err
		}
	}
	return nil
}

func (c *optionalTlsCreds) Clone() credentials.TransportCredentials {
	creds := &optionalTlsCreds{
		tlsForPort: make(map[string]credentials.TransportCredentials),
	}
	for k, v := range c.tlsForPort {
		creds.tlsForPort[k] = v
	}
	return creds
}
