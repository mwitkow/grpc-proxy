
all: test_go

test_go: test.proto
	PATH="${GOPATH}/bin:${PATH}" protoc -I. \
		-I${GOPATH}/src \
		-I${GOPATH}/src/github.com/gogo/protobuf/protobuf/ \
		--go_out=plugins=grpc:. test.proto


