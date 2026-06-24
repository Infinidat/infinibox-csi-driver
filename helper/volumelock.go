/*
Copyright 2026 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helper

import (
	"context"
	"log/slog"
	"sync"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/sets"
)

// VolumeMutex struct
type VolumeMutex struct {
	Mutex *sync.Mutex
}

var singleton *VolumeMutex

var once sync.Once

// GetMutex method
func GetMutex() *VolumeMutex {
	once.Do(func() {
		singleton = &VolumeMutex{Mutex: &sync.Mutex{}}
	})
	return singleton
}

type VolumeLocks struct {
	// deprecated locks sets.String
	locks sets.Set[string]
	mux   sync.Mutex
}

func NewVolumeLocks() *VolumeLocks {
	return &VolumeLocks{
		// deprecated locks: sets.NewString(),
		locks: sets.New[string](),
	}
}

func (vl *VolumeLocks) TryAcquire(volumeID string) bool {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	if vl.locks.Has(volumeID) {
		return false
	}
	vl.locks.Insert(volumeID)
	return true
}

func (vl *VolumeLocks) Release(volumeID string) {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	vl.locks.Delete(volumeID)
}

func LogGRPC(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	slog.Log(ctx, common.LevelTrace, "GRPC call", "method", info.FullMethod)
	slog.Log(ctx, common.LevelTrace, "GRPC request", "value", protosanitizer.StripSecrets(req))

	resp, err := handler(ctx, req)
	if err != nil {
		//slog.Error("GRPC error", "error", err)
		// errors should be reported by the application instead of at the GRPC call level
		// I left these at Trace Level for extreme or weird cases
		slog.Log(ctx, common.LevelTrace, "GRPC error", "error", err.Error())
	} else {
		slog.Log(ctx, common.LevelTrace, "GRPC response", "value", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}
