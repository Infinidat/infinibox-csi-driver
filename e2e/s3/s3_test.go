//go:build e2e

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

package s3test

import (
	"fmt"
	"time"

	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/infinidat/infinibox-csi-driver/e2e"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestS3BucketCreate(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, "s3")
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.SetupNamespace(t.Context(), testConfig)

	bucketClassName := testConfig.TestNames.NSName

	err = createBucketClass(t, testConfig.DynamicClient, bucketClassName)
	if err != nil {
		t.Fatalf("error creating BucketClass %s\n", err.Error())
	}

	bucketClaimName := testConfig.TestNames.NSName
	err = createBucketClaim(t, testConfig.DynamicClient, bucketClassName, bucketClaimName, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("error creating BucketClaim %s\n", err.Error())
	}

	bucketAccessClassName := testConfig.TestNames.NSName
	err = createBucketAccessClass(t, testConfig.DynamicClient, bucketAccessClassName)
	if err != nil {
		t.Fatalf("error creating BucketAccessClass %s\n", err.Error())
	}

	bucketAccessName := testConfig.TestNames.NSName
	secretName := "s3-test"
	err = createBucketAccess(t, testConfig.DynamicClient, bucketAccessName, testConfig.TestNames.NSName, bucketAccessClassName, bucketClaimName, secretName)
	if err != nil {
		t.Fatalf("error creating BucketAccess %s\n", err.Error())
	}

	// wait a bit for the S3 secret to get created by the cosi driver
	t.Log("waiting 5 seconds for S3 secret to be created...")
	time.Sleep(time.Second * 5)

	s, err := testConfig.ClientSet.CoreV1().Secrets(testConfig.TestNames.NSName).Get(t.Context(), secretName, v1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting s3 Secret%s\n", err.Error())
	}
	t.Logf("S3 secret %v\n", s)

	// normally delete the pod which should remove the mpath device from the node after some time
	if *e2e.CleanUp {
		if s != nil {
			err := testConfig.ClientSet.CoreV1().Secrets(testConfig.TestNames.NSName).Delete(t.Context(), secretName, v1.DeleteOptions{})
			if err != nil {
				t.Fatalf("error deleting s3 Secret%s\n", err.Error())
			}
			t.Logf("S3 secret %s deleted\n", s.Name)
		}
		err = e2e.DeleteNamespace(t.Context(), testConfig.TestNames.NSName, testConfig.ClientSet)
		if err != nil {
			t.Logf("error deleting namespace %s\n", err.Error())
		}
		t.Logf("✓ Namespace %s is deleted\n", testConfig.TestNames.NSName)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func createBucketClass(t *testing.T, dynamicClient *dynamic.DynamicClient, bucketClassName string) error {

	// Define the GVR (Group, Version, Resource) for COSI BucketClass
	bucketClassGVR := schema.GroupVersionResource{
		Group:    "objectstorage.k8s.io",
		Version:  "v1alpha1",
		Resource: "bucketclasses",
	}

	// Construct the BucketClass using Unstructured
	bucketClass := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "objectstorage.k8s.io/v1alpha1",
			"kind":       "BucketClass",
			"metadata": map[string]any{
				"name": bucketClassName,
			},
			"driverName": "cosi-driver.infinidat.com",
			"parameters": map[string]any{
				"cosi-driver.infinidat.com/adminSecretName":       "ibox-s3-credentials",
				"cosi-driver.infinidat.com/adminSecretNamespace":  "infinidat-csi",
				"cosi-driver.infinidat.com/accessSecretName":      "ibox-s3-credentials",
				"cosi-driver.infinidat.com/accessSecretNamespace": "infinidat-csi",
			},
			"deletionPolicy": "Delete",
		},
	}

	// Create the resource in the cluster
	t.Log("Creating BucketClass...")
	result, err := dynamicClient.Resource(bucketClassGVR).Create(t.Context(), bucketClass, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	t.Logf("Successfully created BucketClass %s\n", result.GetName())
	return nil
}

func createBucketClaim(t *testing.T, dynamicClient *dynamic.DynamicClient, bucketClassName string, bucketClaimName string, ns string) error {

	// 3. Define the GVR (Group, Version, Resource) for BucketClaim
	bucketClaimGVR := schema.GroupVersionResource{
		Group:    "objectstorage.k8s.io",
		Version:  "v1alpha1",
		Resource: "bucketclaims",
	}

	// 4. Create an unstructured BucketClaim
	bucketClaim := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "objectstorage.k8s.io/v1alpha1",
			"kind":       "BucketClaim",
			"metadata": map[string]any{
				"name":      bucketClaimName,
				"namespace": ns,
			},
			"spec": map[string]any{
				"bucketClassName": bucketClassName,
				"protocols": []any{
					"s3",
				},
			},
		},
	}

	// 5. Apply the BucketClaim to the cluster
	t.Log("Creating BucketClaim...")
	createdBucketClaim, err := dynamicClient.Resource(bucketClaimGVR).Namespace(ns).Create(
		t.Context(),
		bucketClaim,
		metav1.CreateOptions{},
	)
	if err != nil {
		return err
	}

	t.Logf("Successfully created BucketClaim: %s\n", createdBucketClaim.GetName())
	return nil
}

func createBucketAccessClass(t *testing.T, dynamicClient *dynamic.DynamicClient, bacName string) error {

	// 4. Define the GVR (Group, Version, Resource) for BucketAccessClass
	// (Check your specific COSI driver version, typically group is storage.k8s.io/cosi or objectstorage.k8s.io)
	bucketAccessClassGVR := schema.GroupVersionResource{
		Group:    "objectstorage.k8s.io", // e.g., "storage.k8s.io" for newer versions
		Version:  "v1alpha1",
		Resource: "bucketaccessclasses",
	}

	// 5. Define the BucketAccessClass structure
	bucketAccessClass := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "objectstorage.k8s.io/v1alpha1",
			"kind":       "BucketAccessClass",
			"metadata": map[string]any{
				"name": bacName,
			},
			"driverName":         "cosi-driver.infinidat.com",
			"authenticationType": "KEY",
			"parameters": map[string]any{
				"cosi-driver.infinidat.com/adminSecretName":       "ibox-s3-credentials",
				"cosi-driver.infinidat.com/adminSecretNamespace":  "infinidat-csi",
				"cosi-driver.infinidat.com/accessSecretName":      "ibox-s3-credentials",
				"cosi-driver.infinidat.com/accessSecretNamespace": "infinidat-csi",
			},
		},
	}

	// 6. Create the resource in the cluster
	t.Log("Creating BucketAccessClass...")
	result, err := dynamicClient.Resource(bucketAccessClassGVR).Create(
		t.Context(),
		bucketAccessClass,
		metav1.CreateOptions{},
	)
	if err != nil {
		return err
	}

	t.Logf("Successfully created BucketAccessClass: %s\n", result.GetName())
	return nil
}

func createBucketAccess(t *testing.T, dynamicClient *dynamic.DynamicClient, bucketAccessName string, ns string, bucketAccessClassName string, bucketClaimName string, secretName string) error {
	// 3. Define the GroupVersionResource for COSI BucketAccess v1alpha1
	gvr := schema.GroupVersionResource{
		Group:    "objectstorage.k8s.io",
		Version:  "v1alpha1",
		Resource: "bucketaccesses", // Plural form of the Custom Resource
	}

	// 4. Define the BucketAccess Unstructured object
	bucketAccess := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "objectstorage.k8s.io/v1alpha1",
			"kind":       "BucketAccess",
			"metadata": map[string]any{
				"name":      bucketAccessName,
				"namespace": ns,
			},
			"spec": map[string]any{
				"bucketAccessClassName": bucketAccessClassName,
				"bucketClaimName":       bucketClaimName,
				"credentialsSecretName": secretName,
				"protocol":              "s3",
			},
		},
	}

	result, err := dynamicClient.Resource(gvr).Namespace(ns).Create(t.Context(), bucketAccess, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	fmt.Printf("Successfully created BucketAccess: %s\n", result.GetName())
	return nil
}
