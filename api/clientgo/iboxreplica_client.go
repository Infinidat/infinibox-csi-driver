/*
Copyright 2022 Infinidat
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
package clientgo

import (
	"context"
	v1 "infinibox-csi-driver/api/v1"
	"infinibox-csi-driver/log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(v1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	zlog = log.Get()

}

func (kc *kubeclient) GetIboxreplicas() (v1.IboxreplicaList, error) {
	zlog.Info().Msgf("GetIboxreplicas called")
	replicas := v1.IboxreplicaList{}
	crClient, err := crclient.New(kc.restConfig, crclient.Options{Scheme: scheme})
	if err != nil {
		return replicas, err
	}
	err = crClient.List(context.Background(), &replicas)
	if err != nil {
		return replicas, err
	}
	return replicas, nil
}

func (kc *kubeclient) GetIboxreplica(name string) (v1.Iboxreplica, error) {
	zlog.Debug().Msgf("GetIboxreplica %s called", name)
	replica := v1.Iboxreplica{}
	crClient, err := crclient.New(kc.restConfig, crclient.Options{Scheme: scheme})
	if err != nil {
		return replica, err
	}
	key := types.NamespacedName{
		Name: name,
	} // an iboxreplica is a cluster resource so there is no namespace specified
	err = crClient.Get(context.Background(), key, &replica)
	if err != nil {
		return replica, err
	}
	return replica, nil
}

func (kc *kubeclient) CreateIboxreplica(replica v1.Iboxreplica) error {
	zlog.Debug().Msgf("CreateIboxreplica %v called", replica)
	crClient, err := crclient.New(kc.restConfig, crclient.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	err = crClient.Create(context.Background(), &replica)
	if err != nil {
		return err
	}

	return nil
}

func (kc *kubeclient) DeleteIboxreplica(replica v1.Iboxreplica) error {
	zlog.Debug().Msgf("DeleteIboxreplica %v called", replica)
	crClient, err := crclient.New(kc.restConfig, crclient.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	err = crClient.Delete(context.Background(), &replica)
	if err != nil {
		return err
	}
	return nil
}
