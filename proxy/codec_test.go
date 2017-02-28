package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodec_ReadYourWrites(t *testing.T) {
	framePtr := &frame{}
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	codec := rawCodec{}
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
