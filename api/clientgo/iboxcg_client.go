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
package clientgo

import (
	"context"
	"log/slog"

	v1 "github.com/infinidat/infinibox-csi-driver/iboxcg/api/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	schemeForCG = runtime.NewScheme()
)

func init() {
	utilruntime.Must(v1.AddToScheme(schemeForCG))
	//+kubebuilder:scaffold:scheme
}

func (kc *KubeClient) GetIboxcgs(ctx context.Context) (v1.IboxcgList, error) {
	slog.Info("GetIboxpromotes called")
	cgs := v1.IboxcgList{}
	crClient, err := crclient.New(kc.KubeRestConfig, crclient.Options{Scheme: schemeForCG})
	if err != nil {
		return cgs, err
	}
	err = crClient.List(ctx, &cgs)
	if err != nil {
		return cgs, err
	}
	return cgs, nil
}

func (kc *KubeClient) GetIboxcg(ctx context.Context, name string) (v1.Iboxcg, error) {
	slog.Debug("GetIboxcg", "name", name)
	cg := v1.Iboxcg{}
	crClient, err := crclient.New(kc.KubeRestConfig, crclient.Options{Scheme: schemeForCG})
	if err != nil {
		return cg, err
	}
	key := types.NamespacedName{
		Name: name,
	} // an iboxcg is a cluster resource so there is no namespace specified
	err = crClient.Get(ctx, key, &cg)
	if err != nil {
		return cg, err
	}
	return cg, nil
}

func (kc *KubeClient) CreateIboxcg(ctx context.Context, cg v1.Iboxcg) error {
	slog.Debug("CreateIboxcg", "cg", cg)
	crClient, err := crclient.New(kc.KubeRestConfig, crclient.Options{Scheme: schemeForCG})
	if err != nil {
		return err
	}
	err = crClient.Create(ctx, &cg)
	if err != nil {
		return err
	}

	return nil
}

func (kc *KubeClient) DeleteIboxcg(ctx context.Context, cg v1.Iboxcg) error {
	slog.Debug("DeleteIboxcg", "cg", cg)
	crClient, err := crclient.New(kc.KubeRestConfig, crclient.Options{Scheme: schemeForCG})
	if err != nil {
		return err
	}
	err = crClient.Delete(ctx, &cg)
	if err != nil {
		return err
	}
	return nil
}
