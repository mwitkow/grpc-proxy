package sharedflags

import "github.com/spf13/pflag"

var (
	// Set is a common set of flags that are used throughout the libraries and services of grpc proxy.
	// They can be dynamically manipulated through go-flagz
	Set = pflag.NewFlagSet("grpc-proxy", pflag.ExitOnError)
)