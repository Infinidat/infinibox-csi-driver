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

func LogGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	slog.Log(ctx, common.LevelTrace, "GRPC call", "method", info.FullMethod)
	slog.Log(ctx, common.LevelTrace, "GRPC request", "value", protosanitizer.StripSecrets(req))

	resp, err := handler(ctx, req)
	if err != nil {
		slog.Error("GRPC error", "error", err)
	} else {
		slog.Log(ctx, common.LevelTrace, "GRPC response", "value", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}
