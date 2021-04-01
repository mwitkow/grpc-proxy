package proxy

import (
	"flag"
)

var testBackend = flag.String("test-backend", "", "Service providing TestServiceServer")
