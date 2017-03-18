package director

import "testing"
import (
	"github.com/golang/protobuf/jsonpb"
	pb "github.com/mwitkow/grpc-proxy/director/proto"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"google.golang.org/grpc/metadata"

	"github.com/stretchr/testify/assert"
)

func TestRouteMatches(t *testing.T) {
	configJson := `
{ "routes": [
	{
		"backendName": "backendA",
		"serviceNameMatcher": "com.example.a.*"
	},
	{
		"backendName": "backendB_authorityA",
		"serviceNameMatcher": "com.*",
		"authorityMatcher": "authority_a.service.local"
	},
	{
		"backendName": "backendB_authorityB",
		"serviceNameMatcher": "*",
		"authorityMatcher": "authority_b.service.local"
	},
	{
		"backendName": "backendD",
		"serviceNameMatcher": "com.example.",
		"metadataMatcher": {
			"keyOne": "valueOne",
			"keyTwo": "valueTwo"
		}
	},
	{
		"backendName": "backendCatchAllCom",
		"serviceNameMatcher": "com.*"
	}
]}`
	config := &pb.Config{}
	require.NoError(t, jsonpb.UnmarshalString(configJson, config))
	r := &router{config: config}

	for _, tcase := range []struct {
		name            string
		fullServiceName string
		md              metadata.MD
		expectedBackend string
		expectedErr     error
	}{
		{
			name:            "MatchesNoAuthorityJustService",
			fullServiceName: "com.example.a.MyService",
			md:              metadata.Pairs(),
			expectedBackend: "backendA",
			expectedErr:     nil,
		},
		{
			name:            "MatchesAuthorityAndService",
			fullServiceName: "com.example.blah.MyService",
			md:              metadata.Pairs(":authority", "authority_a.service.local"),
			expectedBackend: "backendB_authorityA",
			expectedErr:     nil,
		},
		{
			name:            "MatchesAuthorityAndServiceTakeTwo",
			fullServiceName: "something.else.MyService",
			md:              metadata.Pairs(":authority", "authority_b.service.local"),
			expectedBackend: "backendB_authorityB",
			expectedErr:     nil,
		},
		{
			name:            "MatchesMatchesMetadata",
			fullServiceName: "com.example.whatever.MyService",
			md:              metadata.Pairs("keyOne", "valueOne", "keyTwo", "valueTwo", "keyThree", "somethingUnmatched"),
			expectedBackend: "backendCatchAllCom",
			expectedErr:     nil,
		},
		{
			name:            "MatchesFailsBackToCatchCom_OnBadMetadata",
			fullServiceName: "com.example.whatever.MyService",
			md:              metadata.Pairs("keyTwo", "valueTwo"),
			expectedBackend: "backendCatchAllCom",
			expectedErr:     nil,
		},
		{
			name:            "MatchesFailsBackToCatchCom_OnBadAuthority",
			fullServiceName: "com.example.blah.MyService",
			md:              metadata.Pairs(":authority", "authority_else.service.local"),
			expectedBackend: "backendCatchAllCom",
			expectedErr:     nil,
		},
		{
			name:            "MatchesFailsCompletely_NoBackend",
			fullServiceName: "noncom.else.MyService",
			md:              metadata.Pairs(":authority", "authority_else.service.local"),
			expectedBackend: "",
			expectedErr:     nil,
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			ctx := metadata.NewContext(context.TODO(), tcase.md)
			be, _ := r.Route(tcase.fullServiceName, ctx)
			assert.Equal(t, be, tcase.expectedBackend, "must match expected backend")
		})

	}

}
