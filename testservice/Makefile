
all: test_go

test_go: test.proto
	 protoc --go_out=. --go_opt=paths=source_relative \
        --go-grpc_out=. --go-grpc_opt=paths=source_relative \
        test.proto


