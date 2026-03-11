module api_gateway

go 1.26.0

require (
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/redis/go-redis/v9 v9.17.0
	google.golang.org/grpc v1.79.2
	google.golang.org/protobuf v1.36.11
	stellar v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
)

replace stellar => ../measurement-service
