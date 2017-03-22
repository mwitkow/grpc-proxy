package main

import (
	"bytes"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"

	"github.com/golang/protobuf/proto"
	"github.com/mwitkow/bazel-distcache/common/sharedflags"
	"github.com/mwitkow/go-nicejsonpb"
	pb_be "github.com/mwitkow/grpc-proxy/backendpool/proto"
	pb_director "github.com/mwitkow/grpc-proxy/director/proto"

	"github.com/mwitkow/grpc-proxy/backendpool"
	"github.com/mwitkow/grpc-proxy/director/router"
)

var (
	flagConfigDirectorPath = sharedflags.Set.String(
		"grpcproxy_config_director_path",
		"misc/director.json",
		"Path to the jsonPB file configuring the director.")
	flagConfigBackendPoolPath = sharedflags.Set.String(
		"grpcproxy_config_backendpool_path",
		"misc/backendpool.json",
		"Path to the jsonPB file configuring the backend pool.")
)

func buildRouterOrFail() router.Router {
	cnf := &pb_director.Config{}
	if err := readAsJson(*flagConfigDirectorPath, cnf); err != nil {
		log.Fatalf("failed reading proxy director config: %v", err)
	}
	r := router.NewStatic(cnf)
	return r
}

func buildBackendPoolOrFail() backendpool.Pool {
	cnf := &pb_be.Config{}
	if err := readAsJson(*flagConfigBackendPoolPath, cnf); err != nil {
		log.Fatalf("failed reading backend pool config: %v", err)
	}
	bePool, err := backendpool.NewStatic(cnf)
	if err != nil {
		log.Fatalf("failed creating backend pool: %v", err)
	}
	return bePool
}

func readAsJson(filePath string, destination proto.Message) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	um := &nicejsonpb.Unmarshaler{AllowUnknownFields: false}
	err = um.Unmarshal(bytes.NewReader(data), destination)
	if err != nil {
		return err
	}
	return nil
}
