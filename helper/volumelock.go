package helper

import (
	"context"
	"sync"

	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
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
	locks sets.String
	mux   sync.Mutex
}

func NewVolumeLocks() *VolumeLocks {
	return &VolumeLocks{
		locks: sets.NewString(),
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
	//level := klog.Level(getLogLevel(info.FullMethod))
	klog.V(8).Infof("GRPC call: %s", info.FullMethod)
	klog.V(8).Infof("GRPC request: %s", protosanitizer.StripSecrets(req))

	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf("GRPC error: %v", err)
	} else {
		klog.V(8).Infof("GRPC response: %s", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}

/**
func getLogLevel(method string) int32 {
	if method == "/csi.v1.Identity/Probe" ||
		method == "/csi.v1.Node/NodeGetCapabilities" ||
		method == "/csi.v1.Node/NodeGetVolumeStats" {
		return 8
	}
	return 2
}
*/
