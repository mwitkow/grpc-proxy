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
// limitations under the License.`

package proxy
import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// StreamDirector returns a gRPC ClientConn for a stream of a given context.
// The service name, method name, and other `MD` metadata (e.g. authority) can be extracted from Context.
type StreamDirector func(ctx context.Context) (*grpc.ClientConn, error)
