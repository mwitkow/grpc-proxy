# Proof of concept Server

This server starts up a gRPC reverse proxy.

## Configuration

Driven through two config files: 

[backendpool.json](misc/backendpool.json):
```json
{
  "backends": [
    {
      "name": "controller",
      "balancer": "ROUND_ROBIN",
      "interceptors": [
        { "prometheus": true }

      ],
      "srv": {
        "dns_name": "controller.eu1-prod.internal.improbable.io"
      }
    }
  ]
}
```

[director.json](misc/director.json):
```json
{
  "routes": [
    {
      "backend_name": "controller",
      "service_name_matcher": "*",
      "authority_matcher": "controller.eu1-prod.improbable.local"
    }
  ]
}
```

## Running:

Here's an example that runs the server listening on four ports (80 for debug HTTP, 443 for HTTPS+gRPCTLS, 444 for gRPCTLS, 81 for gRPC plain text), and requiring 
client side certs:
```sh
go build 
./server \
  --server_grpc_port=81 \
  --server_grpc_tls_port=444 \
  --server_http_port=80 \
  --server_http_tls_port=443 \ 
  --server_tls_cert_file=misc/localhost.crt \ 
  --server_tls_key_file=misc/localhost.key \
  --server_tls_client_ca_files=misc/ca.crt \ 
  --server_tls_client_cert_required=true
```