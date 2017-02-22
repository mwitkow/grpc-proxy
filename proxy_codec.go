package proxy

import (
	"fmt"
	"google.golang.org/grpc"
	"github.com/golang/protobuf/proto"
)

// ProxyCodec is custom codec for gRPC server that a no-op codec if the unmarshalling is done to/from bytes.
// This is required for proxy functionality (as the proxy doesn't know the types). But in case of methods implemented
// on the server, it falls back to the proto codec.
func ProxyCodec() grpc.ServerOption {
	return grpc.CustomCodec(&codec{&protoCodec{}})
}

func WithProxyCodec() grpc.DialOption {
	return grpc.WithCodec(&codec{&protoCodec{}})
}

type codec struct {
	parentCodec grpc.Codec
}

type frame struct {
	payload []byte
}

func (c *codec) Marshal(v interface{}) ([]byte, error) {
	out, ok := v.(*frame)
	if !ok {
		return c.parentCodec.Marshal(v)
	}
	return out.payload, nil

}

func (c *codec) Unmarshal(data []byte, v interface{}) error {
	dst, ok := v.(*frame)
	if !ok {
		return c.parentCodec.Unmarshal(data, v)
	}
	dst.payload = data
	return nil
}

func (c *codec) String() string {
	return fmt.Sprintf("proxy>%s", c.parentCodec.String())
}

// protoCodec is a Codec implementation with protobuf. It is the default codec for gRPC.
type protoCodec struct{}

func (protoCodec) Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (protoCodec) Unmarshal(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}

func (protoCodec) String() string {
	return "proto"
}
