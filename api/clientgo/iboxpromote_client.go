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

	v1 "github.com/infinidat/infinibox-csi-driver/iboxpromote/api/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	schemeForPromote = runtime.NewScheme()
)

func init() {
	utilruntime.Must(v1.AddToScheme(schemeForPromote))
	//+kubebuilder:scaffold:scheme
}

func (kc *KubeClient) GetIboxpromotes(ctx context.Context) (v1.IboxpromoteList, error) {
	slog.Info("GetIboxpromotes called")
	promotes := v1.IboxpromoteList{}
	crClient, err := crclient.New(kc.KubeRestConfig, crclient.Options{Scheme: schemeForPromote})
	if err != nil {
		return promotes, err
	}
	err = crClient.List(ctx, &promotes)
	if err != nil {
		return promotes, err
	}
	return promotes, nil
}

func (kc *KubeClient) GetIboxpromote(ctx context.Context, name string) (v1.Iboxpromote, error) {
	slog.Debug("GetIboxpromote", "name", name)
	promote := v1.Iboxpromote{}
	crClient, err := crclient.New(kc.KubeRestConfig, crclient.Options{Scheme: schemeForPromote})
	if err != nil {
		return promote, err
	}
	key := types.NamespacedName{
		Name: name,
	} // an iboxpromote is a cluster resource so there is no namespace specified
	err = crClient.Get(ctx, key, &promote)
	if err != nil {
		return promote, err
	}
	return promote, nil
}

func (kc *KubeClient) CreateIboxpromote(ctx context.Context, promote v1.Iboxpromote) error {
	slog.Debug("CreateIboxpromote", "promote", promote)
	crClient, err := crclient.New(kc.KubeRestConfig, crclient.Options{Scheme: schemeForPromote})
	if err != nil {
		return err
	}
	err = crClient.Create(ctx, &promote)
	if err != nil {
		return err
	}

	return nil
}

func (kc *KubeClient) DeleteIboxpromote(ctx context.Context, promote v1.Iboxpromote) error {
	slog.Debug("DeleteIboxpromote", "promote", promote)
	crClient, err := crclient.New(kc.KubeRestConfig, crclient.Options{Scheme: schemeForPromote})
	if err != nil {
		return err
	}
	err = crClient.Delete(ctx, &promote)
	if err != nil {
		return err
	}
	return nil
}
