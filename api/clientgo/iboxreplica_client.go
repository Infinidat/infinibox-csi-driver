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
	"log/slog"

	v1 "github.com/infinidat/infinibox-csi-driver/iboxreplica/api/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	schemeForReplica = runtime.NewScheme()
)

func init() {
	utilruntime.Must(v1.AddToScheme(schemeForReplica))
	//+kubebuilder:scaffold:scheme
}

func (kc *kubeclient) GetIboxreplicas(ctx context.Context) (v1.IboxreplicaList, error) {
	slog.Info("GetIboxreplicas called")
	replicas := v1.IboxreplicaList{}
	crClient, err := crclient.New(kc.restConfig, crclient.Options{Scheme: schemeForReplica})
	if err != nil {
		return replicas, err
	}
	err = crClient.List(ctx, &replicas)
	if err != nil {
		return replicas, err
	}
	return replicas, nil
}

func (kc *kubeclient) GetIboxreplica(ctx context.Context, name string) (v1.Iboxreplica, error) {
	slog.Debug("GetIboxreplica", "name", name)
	replica := v1.Iboxreplica{}
	crClient, err := crclient.New(kc.restConfig, crclient.Options{Scheme: schemeForReplica})
	if err != nil {
		return replica, err
	}
	key := types.NamespacedName{
		Name: name,
	} // an iboxreplica is a cluster resource so there is no namespace specified
	err = crClient.Get(ctx, key, &replica)
	if err != nil {
		return replica, err
	}
	return replica, nil
}

func (kc *kubeclient) CreateIboxreplica(ctx context.Context, replica v1.Iboxreplica) error {
	slog.Debug("CreateIboxreplica", "replica", replica)
	crClient, err := crclient.New(kc.restConfig, crclient.Options{Scheme: schemeForReplica})
	if err != nil {
		return err
	}
	err = crClient.Create(ctx, &replica)
	if err != nil {
		return err
	}

	return nil
}

func (kc *kubeclient) DeleteIboxreplica(ctx context.Context, replica v1.Iboxreplica) error {
	slog.Debug("DeleteIboxreplica", "replica", replica)
	crClient, err := crclient.New(kc.restConfig, crclient.Options{Scheme: schemeForReplica})
	if err != nil {
		return err
	}
	err = crClient.Delete(ctx, &replica)
	if err != nil {
		return err
	}
	return nil
}
