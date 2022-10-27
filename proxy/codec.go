package proxy

import (
	"fmt"
	"google.golang.org/grpc/encoding"

	"google.golang.org/protobuf/proto"
)

// Codec returns a proxying encoding.Codec with the default protobuf codec as parent.

func Codec() encoding.Codec {
	return CodecWithParent(&protoCodec{})
}

// CodecWithParent returns a proxying encoding.Codec with a user provided codec as parent.
func CodecWithParent(fallback encoding.Codec) encoding.Codec {
	return &rawCodec{fallback}
}

type rawCodec struct {
	parentCodec encoding.Codec
}

func (c *rawCodec) Name() string {
	return "proto"
}

type frame struct {
	payload []byte
}

func (c *rawCodec) Marshal(v interface{}) ([]byte, error) {
	out, ok := v.(*frame)
	if !ok {
		return c.parentCodec.Marshal(v)
	}
	return out.payload, nil

}

func (c *rawCodec) Unmarshal(data []byte, v interface{}) error {
	dst, ok := v.(*frame)
	if !ok {
		return c.parentCodec.Unmarshal(data, v)
	}
	dst.payload = data
	return nil
}

func (c *rawCodec) String() string {
	return fmt.Sprintf("proxy>%s", c.parentCodec.Name())
}

// protoCodec is a Codec implementation with protobuf. It is the default rawCodec for gRPC.
type protoCodec struct{}

func (c protoCodec) Name() string {
	return "proto"
}

func (protoCodec) Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (protoCodec) Unmarshal(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}
