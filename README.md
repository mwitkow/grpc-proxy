# gRPC Proxy

This is an implementation of a [gRPC](http://www.grpc.io/) Proxying Server in Golang, based on [grpc-go](https://github.com/grpc/grpc-go). Features:

 * full support for all Streams: Unitary RPCs and Streams: One-Many, Many-One, Many-Many
 * pass-through mode: no overhead of encoding/decoding messages
 * customizable `StreamDirector` routing based on `context.Context` of the `Stream`, allowing users to return
   a `grpc.ClientConn` after dialing the backend of choice based on:
     - inspection of service and method name
     - inspection of user credentials in `authorization` header
     - inspection of custom user-features
     - inspection of TLS client cert credentials
 * integration tests
 
## Example Use
 
```go

director := func(ctx context.Context) (*grpc.ClientConn, error) {
    if err := CheckBearerToken(ctx); err != nil {
        return nil, grpc.Errorf(codes.PermissionDenied, "unauthorized access: %v", err)
    }
    stream, _ := transport.StreamFromContext(ctx)
    backend, found := PreDialledBackends[stream.Method()];
    if !found {
        return nil, grpc.Errorf(codes.Unimplemented, "the service %v is not implemented", stream.Method)
    }
    return backend, nil
}

proxy := grpcproxy.NewProxy(director)
proxy.Server(boundListener)
```

## Status

This is *alpha* software, written as a proof of concept. It has been integration-tested, but please expect bugs.

The current implementation depends on a public interface to `ClientConn.Picker()`, which hopefully will be upstreamed in [grpc-go#397](https://github.com/grpc/grpc-go/pull/397).
   

## Contributors

Names in no particular order:

* [mwitkow](https://github.com/mwitkow)

## License

`grpc-proxy` is released under the Apache 2.0 license. See [LICENSE.txt](https://github.com/spf13/mwitkow-io/blob/grpcproxy/LICENSE.txt).


Part of the main server loop are lifted from the [grpc-go](https://github.com/grpc/grpc-go) `Server`, which is copyrighted Google Inc. and licensed under MIT license.
