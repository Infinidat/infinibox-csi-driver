package main

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

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

/**
required environment variables:
	"S3_CREDENTIALS_SECRET_NAME" - this is the name of the ibox S3 secret (used to lookup the secret)
	"POD_NAMESPACE" - the namespace of the S3 secret (used to lookup the secret)
	"BUCKET_NAME" - the name of an existing bucket
	"KUBECONFIG" (optional) - the kubeconfig path if you wanted to run the client outside of a Pod for testing
*/

const (
	objectKey = "example-file.txt" // Destination path inside S3

	AWS_ACCESS_KEY_ID     = "AWS_ACCESS_KEY_ID"
	AWS_SECRET_ACCESS_KEY = "AWS_SECRET_ACCESS_KEY"
	AWS_ENDPOINT_URL      = "AWS_ENDPOINT_URL"

	S3_CREDENTIALS_SECRET_NAME = "S3_CREDENTIALS_SECRET_NAME"
	POD_NAMESPACE              = "POD_NAMESPACE"
	BUCKET_NAME                = "BUCKET_NAME"
)

type S3Credentials struct {
	AccessKey       string
	SecretAccessKey string
	Endpoint        string
}

func main() {
	slog.Info("Start")
	ctx := context.Background()

	creds, err := getS3Credentials()
	if err != nil {
		slog.Error("unable to get secret", "error", err)
		os.Exit(1)
	}

	slog.Info("got credentials...")

	// read the ibox S3 secret to get the S3 credentials
	// we need the access_key, secret_access_key, and endpoint to
	// connect to the ibox S3 service

	err = os.Setenv(AWS_ACCESS_KEY_ID, creds.AccessKey)
	if err != nil {
		slog.Error("error setting env var", "value", AWS_ACCESS_KEY_ID, "error", err)
		os.Exit(1)

	}
	err = os.Setenv(AWS_SECRET_ACCESS_KEY, creds.SecretAccessKey)
	if err != nil {
		slog.Error("error setting env var", "value", AWS_SECRET_ACCESS_KEY, "error", err)
		os.Exit(1)

	}
	bucketName := os.Getenv(BUCKET_NAME)
	if bucketName == "" {
		slog.Error("env var is not set and is a required env var", "value", BUCKET_NAME)
		os.Exit(1)
	}

	// 1. Load the default AWS configuration (~/.aws/config or Env Vars)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		slog.Error("unable to load SDK config", "error", err)
		os.Exit(1)
	}
	cfg.BaseEndpoint = aws.String(creds.Endpoint)

	// 2. Initialize the Amazon S3 client
	s3Client := s3.NewFromConfig(cfg)

	// 3. Upload data to S3
	content := "Hello, from infinidat-cosi-test-client!"
	slog.Info("Uploading content to bucket", "bucket", bucketName, "key", objectKey)

	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader([]byte(content)),
	})
	if err != nil {
		slog.Error("failed to upload object", "error", err)
		os.Exit(1)
	}
	slog.Info("Upload successful!")

	for {
		// 4. Download the object back from S3
		slog.Info("Downloading object", "key", objectKey)
		output, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectKey),
		})
		if err != nil {
			slog.Error("failed to download object", "error", err)
			os.Exit(1)
		}

		// Read and print the file data
		data, err := io.ReadAll(output.Body)
		if err != nil {
			slog.Error("failed to read object body", "error", err)
			os.Exit(1)
		}

		slog.Info("Downloaded content", "content", string(data))

		//defer output.Body.Close()
		err = output.Body.Close()
		if err != nil {
			slog.Error("error closing body", "error", err)
		}
		slog.Info("sleeping 5 minutes before next GetObject call!")
		time.Sleep(5 * time.Minute)
	}
}

func getS3Credentials() (creds *S3Credentials, err error) {
	var config *rest.Config

	ns := os.Getenv(POD_NAMESPACE)
	if ns == "" {
		return nil, fmt.Errorf("%s not set", POD_NAMESPACE)
	}

	secretName := os.Getenv(S3_CREDENTIALS_SECRET_NAME)
	if secretName == "" {
		return nil, fmt.Errorf("%s not set", S3_CREDENTIALS_SECRET_NAME)
	}

	// 1. Try inside-the-cluster authentication first
	config, err = rest.InClusterConfig()
	if err != nil {
		slog.Info("Not running inside a cluster, falling back to kubeconfig...")

		// 2. Try loading via KUBECONFIG env variable or default home path
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			homeDir, _ := os.UserHomeDir()
			kubeconfigPath = filepath.Join(homeDir, ".kube", "config")
		}

		// Build config from the resolved local path
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}

	// 3. Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	secret, err := clientset.CoreV1().Secrets(ns).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	creds = &S3Credentials{
		AccessKey:       string(secret.Data[AWS_ACCESS_KEY_ID]),
		SecretAccessKey: string(secret.Data[AWS_SECRET_ACCESS_KEY]),
		Endpoint:        string(secret.Data[AWS_ENDPOINT_URL]),
	}
	if creds.AccessKey == "" {
		return nil, fmt.Errorf("%s missing in S3 secret", AWS_ACCESS_KEY_ID)
	}
	if creds.SecretAccessKey == "" {
		return nil, fmt.Errorf("%s missing in S3 secret", AWS_SECRET_ACCESS_KEY)
	}
	if creds.Endpoint == "" {
		return nil, fmt.Errorf("%s missing in S3 secret", AWS_ENDPOINT_URL)
	}
	return creds, nil
}
