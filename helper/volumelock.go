package helper

import (
	"sync"
)

//VolumeMutex struct
type VolumeMutex struct {
	Mutex *sync.Mutex
	
}

var singleton *VolumeMutex

var once sync.Once

//GetMutex method
func GetMutex() *VolumeMutex {
    once.Do(func() {
        singleton = &VolumeMutex{Mutex:&sync.Mutex{}}
    })
    return singleton
}
