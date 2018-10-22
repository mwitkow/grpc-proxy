package codec_test

import (
	"testing"

	_ "github.com/gogo/protobuf/proto"
	codec "github.com/mwitkow/grpc-proxy/proxy/codec"
	pb "github.com/mwitkow/grpc-proxy/testservice"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/encoding"
)

func TestCodec_ReadYourWrites(t *testing.T) {
	framePtr := &codec.Frame{}
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	codec.Register()
	codec := encoding.GetCodec((&codec.Proxy{}).Name())
	require.NotNil(t, codec, "codec must be registered")
	require.NoError(t, codec.Unmarshal(data, framePtr), "unmarshalling must go ok")
	out, err := codec.Marshal(framePtr)
	require.NoError(t, err, "no marshal error")
	require.Equal(t, data, out, "output and data must be the same")

	// reuse
	require.NoError(t, codec.Unmarshal([]byte{0x55}, framePtr), "unmarshalling must go ok")
	out, err = codec.Marshal(framePtr)
	require.NoError(t, err, "no marshal error")
	require.Equal(t, []byte{0x55}, out, "output and data must be the same")

}

func TestProtoCodec_ReadYourWrites(t *testing.T) {
	p1 := &pb.PingRequest{
		Value: "test-ping",
	}
	proxyCd := encoding.GetCodec((&codec.Proxy{}).Name())

	require.NotNil(t, proxyCd, "proxy codec must not be nil")

	out1p1, err := proxyCd.Marshal(p1)
	require.NoError(t, err, "marshalling must go ok")
	out2p1, err := proxyCd.Marshal(p1)
	require.NoError(t, err, "marshalling must go ok")

	p2 := &pb.PingRequest{}
	err = proxyCd.Unmarshal(out1p1, p2)
	require.NoError(t, err, "unmarshalling must go ok")
	err = proxyCd.Unmarshal(out2p1, p2)
	require.NoError(t, err, "unmarshalling must go ok")

	require.Equal(t, *p1, *p2)
}
