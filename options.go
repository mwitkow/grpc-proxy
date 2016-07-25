// Copyright © 2015 Michal Witkowski <michal@improbable.io>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy
import (
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
)

type options struct {
	creds                credentials.TransportCredentials
	logger               grpclog.Logger
	maxConcurrentStreams uint32
}

// A ProxyOption sets options.
type ProxyOption func(*options)


// UsingLogger returns a ProxyOption that makes use of a logger other than the default `grpclogger`.
func UsingLogger(logger grpclog.Logger) ProxyOption {
	return func(o *options) {
		o.logger = logger
	}
}

// MaxConcurrentStreams returns a ProxyOption that will apply a limit on the number
// of concurrent streams to each ServerTransport.
func MaxConcurrentStreams(n uint32) ProxyOption {
	return func(o *options) {
		o.maxConcurrentStreams = n
	}
}

// Creds returns a ProxyOption that sets credentials for server connections.
func Creds(c credentials.TransportCredentials) ProxyOption {
	return func(o *options) {
		o.creds = c
	}
}


type defaultLogger struct{}

func (g *defaultLogger) Fatal(args ...interface{}) {
	grpclog.Fatal(args...)
}

func (g *defaultLogger) Fatalf(format string, args ...interface{}) {
	grpclog.Fatalf(format, args...)
}

func (g *defaultLogger) Fatalln(args ...interface{}) {
	grpclog.Fatalln(args...)
}

func (g *defaultLogger) Print(args ...interface{}) {
	grpclog.Print(args...)
}

func (g *defaultLogger) Printf(format string, args ...interface{}) {
	grpclog.Printf(format, args...)
}

func (g *defaultLogger) Println(args ...interface{}) {
	grpclog.Println(args...)
}
