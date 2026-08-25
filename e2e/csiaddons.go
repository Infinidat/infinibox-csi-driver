/*
Copyright 2026 infinidat

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

package e2e

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//replicationv1alpha1 "github.com/csi-addons/spec/lib/go/replication"
	csiaddonsv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/csiaddons/v1alpha1"
	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
)

// CreateVolumeReplicationClass creates a VolumeReplicationClass
func CreateVolumeReplicationClass(addonsClient client.Client, t *testing.T, testConfig *TestConfig, name string, provisioner string, parameters map[string]string) (*replicationv1alpha1.VolumeReplicationClass, error) {
	t.Logf("Creating VolumeReplicationClass %s", name)

	vrc := &replicationv1alpha1.VolumeReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: replicationv1alpha1.VolumeReplicationClassSpec{
			Provisioner: provisioner,
			Parameters:  parameters,
		},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	err := addonsClient.Create(ctx, vrc)
	if err != nil {
		return nil, err
	}

	return vrc, nil
}

// DeleteVolumeReplicationClass deletes a VolumeReplicationClass
func DeleteVolumeReplicationClass(addonsClient client.Client, t *testing.T, testConfig *TestConfig, vrc *replicationv1alpha1.VolumeReplicationClass) error {
	t.Logf("Deleting VolumeReplicationClass %s", vrc.Name)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	err := addonsClient.Delete(ctx, vrc, &client.DeleteOptions{})
	if err != nil {
		return err
	}

	return nil
}

// CreateVolumeReplication creates a VolumeReplication
func CreateVolumeReplication(addonsClient client.Client, t *testing.T, testConfig *TestConfig, name string, pvcName string, vrcName string, replicationState replicationv1alpha1.ReplicationState) (*replicationv1alpha1.VolumeReplication, error) {
	t.Logf("Creating VolumeReplication %s for PVC %s", name, pvcName)
	t1 := metav1.NewTime(time.Now())

	vr := &replicationv1alpha1.VolumeReplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testConfig.TestNames.NSName,
		},
		Spec: replicationv1alpha1.VolumeReplicationSpec{
			VolumeReplicationClass: vrcName,
			ReplicationState:       replicationState,
			DataSource: corev1.TypedLocalObjectReference{
				Kind: "PersistentVolumeClaim",
				Name: pvcName,
			},
		},
		Status: replicationv1alpha1.VolumeReplicationStatus{
			LastSyncTime: &t1,
		},
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	err := addonsClient.Create(ctx, vr)
	if err != nil {
		return nil, err
	}

	return vr, nil
}

// DeleteVolumeReplication deletes a VolumeReplication
func DeleteVolumeReplication(addonsClient client.Client, t *testing.T, testConfig *TestConfig, vr *replicationv1alpha1.VolumeReplication) error {
	t.Logf("Deleting VolumeReplication %s/%s", vr.Namespace, vr.Name)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	err := addonsClient.Delete(ctx, vr, &client.DeleteOptions{})
	if err != nil {
		return err
	}

	return nil
}

// WaitForVolumeReplicationState waits for a VolumeReplication to reach a specific state
func WaitForVolumeReplicationState(t *testing.T, testConfig *TestConfig, name string, expectedState replicationv1alpha1.State) (*replicationv1alpha1.VolumeReplication, error) {
	t.Logf("Waiting for VolumeReplication %s to reach state %s", name, expectedState)

	vr := &replicationv1alpha1.VolumeReplication{}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	s := runtime.NewScheme()
	err := scheme.AddToScheme(s)
	if err != nil {
		return nil, err
	}
	err = csiaddonsv1alpha1.AddToScheme(s)
	if err != nil {
		return nil, err
	}
	err = replicationv1alpha1.AddToScheme(s)
	if err != nil {
		return nil, err
	}

	// Create controller-runtime client
	addonsClient, err := client.New(testConfig.RestConfig, client.Options{Scheme: s})
	if err != nil {
		return nil, err
	}

	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, time.Second*20, true, func(ctx context.Context) (bool, error) {
		err := addonsClient.Get(ctx, client.ObjectKey{
			Name:      name,
			Namespace: testConfig.TestNames.NSName,
		}, vr)
		if err != nil {
			return false, err
		}
		return vr.Status.State == expectedState, nil
	})

	if err != nil {
		return nil, err
	}
	return vr, nil
}

// GetVolumeReplication gets a VolumeReplication
func GetVolumeReplication(t *testing.T, testConfig *TestConfig, name string) (*replicationv1alpha1.VolumeReplication, error) {
	vr := &replicationv1alpha1.VolumeReplication{}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	s := runtime.NewScheme()
	err := scheme.AddToScheme(s)
	if err != nil {
		return nil, err
	}
	err = csiaddonsv1alpha1.AddToScheme(s)
	if err != nil {
		return nil, err
	}
	err = replicationv1alpha1.AddToScheme(s)
	if err != nil {
		return nil, err
	}

	// Create controller-runtime client
	addonsClient, err := client.New(testConfig.RestConfig, client.Options{Scheme: s})
	if err != nil {
		return nil, err
	}
	err = addonsClient.Get(ctx, client.ObjectKey{
		Name:      name,
		Namespace: testConfig.TestNames.NSName,
	}, vr)

	if err != nil {
		return nil, err
	}
	return vr, nil
}

// UpdateVolumeReplication updates a VolumeReplication
func UpdateVolumeReplication(t *testing.T, testConfig *TestConfig, vr *replicationv1alpha1.VolumeReplication) error {
	t.Logf("Updating VolumeReplication %s", vr.Name)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	s := runtime.NewScheme()
	err := scheme.AddToScheme(s)
	if err != nil {
		return err
	}
	err = csiaddonsv1alpha1.AddToScheme(s)
	if err != nil {
		return err
	}
	err = replicationv1alpha1.AddToScheme(s)
	if err != nil {
		return err
	}

	// Create controller-runtime client
	addonsClient, err := client.New(testConfig.RestConfig, client.Options{Scheme: s})
	if err != nil {
		return err
	}
	err = addonsClient.Update(ctx, vr)
	if err != nil {
		return err
	}
	return nil
}

// CreateVolumeGroupReplicationClass creates a VolumeGroupReplicationClass
func CreateVolumeGroupReplicationClass(addonsClient client.Client, t *testing.T, testConfig *TestConfig, name string, provisioner string, parameters map[string]string) (*replicationv1alpha1.VolumeGroupReplicationClass, error) {
	t.Logf("Creating VolumeGroupReplicationClass %s", name)

	vgrc := &replicationv1alpha1.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: replicationv1alpha1.VolumeGroupReplicationClassSpec{
			Provisioner: provisioner,
			Parameters:  parameters,
		},
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	err := addonsClient.Create(ctx, vgrc)
	if err != nil {
		return nil, err
	}

	return vgrc, nil
}

// GetVolumeGroupReplicationClass gets a VolumeGroupReplicationClass
func GetVolumeGroupReplicationClass(addonsClient client.Client, t *testing.T, testConfig *TestConfig, name string) (*replicationv1alpha1.VolumeGroupReplicationClass, error) {
	vgrc := &replicationv1alpha1.VolumeGroupReplicationClass{}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	err := addonsClient.Get(ctx, client.ObjectKey{Name: name}, vgrc)
	if err != nil {
		return nil, err
	}
	return vgrc, nil
}

// DeleteVolumeGroupReplicationClass deletes a VolumeGroupReplicationClass
func DeleteVolumeGroupReplicationClass(addonsClient client.Client, t *testing.T, vgrc *replicationv1alpha1.VolumeGroupReplicationClass) error {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*8)
	defer cancel()

	err := addonsClient.Delete(ctx, vgrc, &client.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}
