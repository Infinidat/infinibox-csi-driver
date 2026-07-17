package s3

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
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
)

const (
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

const (
	ParamAdminSecretName      = "cosi-driver.infinidat.com/adminSecretName"
	ParamAdminSecretNamespace = "cosi-driver.infinidat.com/adminSecretNamespace"

	ParamAccessSecretName      = "cosi-driver.infinidat.com/accessSecretName"
	ParamAccessSecretNamespace = "cosi-driver.infinidat.com/accessSecretNamespace"
)

// Factory is responsible for creating instances of the S3 client.
type Factory struct {
	kubeCli kubernetes.Interface
}

// NewFactory creates a new Factory instance with the provided Kubernetes client.
func NewFactory(kubeCli kubernetes.Interface) *Factory {
	return &Factory{kubeCli: kubeCli}
}

// DynamicClient defines the interface for interacting with the S3 service.
type DynamicClient interface {
	BucketInfo(ctx context.Context, bucket string) (*BucketInfo, error)

	CreateBucket(ctx context.Context, bucket string, params map[string]string) (*BucketInfo, error)
	DeleteBucket(ctx context.Context, bucket string, params map[string]string) error

	CreateBucketAccess(ctx context.Context, userID string, buckets []string, params map[string]string) (*AccessInfo, error)
	DeleteBucketAccess(ctx context.Context, userID string, buckets []string, params map[string]string) error
}

// NewClient creates a new S3 client instance.
func (f *Factory) NewClient(ctx context.Context, params map[string]string) (DynamicClient, error) {
	adminSecretName, ok := params[ParamAdminSecretName]
	if !ok {
		return nil, errMissingParameter(ParamAdminSecretName)
	}

	adminSecretNamespace, ok := params[ParamAdminSecretNamespace]
	if !ok {
		return nil, errMissingParameter(ParamAdminSecretNamespace)
	}

	secret, err := f.kubeCli.CoreV1().Secrets(adminSecretNamespace).Get(ctx, adminSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve admin secret: %w", err)
	}

	cfg, endpoint, err := LoadAWSConfigFromSecret(ctx, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config from secret: %w", err)
	}

	s3cli := s3.NewFromConfig(cfg,
		func(o *s3.Options) {
			o.BaseEndpoint = new(endpoint)
			o.UsePathStyle = true
			o.Region = "us-east-1"
		},
	)

	return &Client{
		s3cli:    s3cli,
		endpoint: endpoint,
		//region:   cfg.Region,
		region:  "us-east-1",
		kubeCli: f.kubeCli,
	}, nil
}

// LoadAWSConfigFromSecret loads AWS configuration from a Kubernetes secret.
func LoadAWSConfigFromSecret(ctx context.Context, secret *v1.Secret) (aws.Config, string, error) {
	accessKeyID, ok := secret.Data[AWS_ACCESS_KEY_ID]
	if !ok {
		return aws.Config{}, "", fmt.Errorf("missing %s in secret %s/%s", AWS_ACCESS_KEY_ID, secret.Namespace, secret.Name)
	}

	secretAccessKey, ok := secret.Data[AWS_SECRET_ACCESS_KEY]
	if !ok {
		return aws.Config{}, "", fmt.Errorf("missing %s in secret %s/%s", AWS_SECRET_ACCESS_KEY, secret.Namespace, secret.Name)
	}

	endpoint, ok := secret.Data[AWS_ENDPOINT_URL]
	if !ok {
		return aws.Config{}, "", fmt.Errorf("missing %s in secret %s/%s", AWS_ENDPOINT_URL, secret.Namespace, secret.Name)
	}

	region := secret.Data["AWS_REGION"] // optional

	tmp := os.Getenv("S3_INSECURE_SKIP_VERIFY")
	insecureSkipVerify, err := strconv.ParseBool(tmp)
	if err != nil {
		slog.Info("error getting S3_INSECURE_SKIP_VERIFY env var, using default of true")
		insecureSkipVerify = true // the default
	}

	// 1. Create a custom HTTP Transport that skips TLS verification
	customTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify, // Disables SSL/TLS verification
		},
	}

	// 2. Wrap it inside a standard http.Client
	httpClient := &http.Client{
		Transport: customTransport,
	}

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(string(region)),
		config.WithHTTPClient(httpClient),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				string(accessKeyID),
				string(secretAccessKey),
				"",
			),
		),
	)
	if err != nil {
		return aws.Config{}, "", err
	}

	return cfg, string(endpoint), nil
}

// Client represents an S3 client instance.
// It provides methods for performing operations on S3 buckets and managing user access.
type Client struct {
	s3cli    *s3.Client
	endpoint string
	region   string
	kubeCli  kubernetes.Interface
}

// resolveRegion returns the region reported by the backend, falling back to
// the region configured on the client. S3-compatible backends (e.g. Ceph RGW)
// frequently omit the BucketRegion response header, which leaves the COSI
// response with an empty region and causes the sidecar to reject it.
func (c *Client) resolveRegion(headRegion *string) string {
	if r := ptr.Deref(headRegion, ""); r != "" {
		return r
	}
	slog.Info("setting region...")
	return "us-east-1"
	//return c.region
}

// interface guard
var _ DynamicClient = (*Client)(nil)

// BucketInfo checks if a bucket exists in the S3 service and retrieves its metadata.
func (c *Client) BucketInfo(ctx context.Context, bucket string) (*BucketInfo, error) {
	slog.Info("BucketInfo is called", "bucket", bucket)
	req := &s3.HeadBucketInput{Bucket: new(bucket)}

	head, err := c.s3cli.HeadBucket(ctx, req)
	if err != nil {
		if apiError, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiError.(type) {
			case *types.NotFound:
				return nil, errBucketNotFound(bucket)
			}
		}

		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}

	return &BucketInfo{
		BucketName: bucket,
		Endpoint:   c.endpoint,
		Region:     c.resolveRegion(head.BucketRegion),
	}, nil
}

// BucketInfo represents metadata about an S3 bucket, such as its name, creation date, and other relevant information.
type BucketInfo struct {
	BucketName string
	Endpoint   string
	Region     string
}

// CreateBucket creates a new bucket in the S3 service.
func (c *Client) CreateBucket(ctx context.Context, bucket string, _ map[string]string) (*BucketInfo, error) {
	_, err := c.s3cli.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: new(bucket),
	})
	if err != nil {
		if apiError, ok := errors.AsType[smithy.APIError](err); ok {
			switch apiError.(type) {
			case *types.BucketAlreadyExists, *types.BucketAlreadyOwnedByYou:
				// nothing to do, we hit a race
			default:
				return nil, fmt.Errorf("failed to create bucket: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	head, err := c.s3cli.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: new(bucket),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve bucket info after creation: %w", err)
	}

	return &BucketInfo{
		BucketName: bucket,
		Endpoint:   c.endpoint,
		Region:     c.resolveRegion(head.BucketRegion),
	}, nil
}

// DeleteBucket deletes a bucket from the S3 service.
func (c *Client) DeleteBucket(ctx context.Context, bucket string, _ map[string]string) error {
	req := &s3.DeleteBucketInput{
		Bucket: new(bucket),
	}

	_, err := c.s3cli.DeleteBucket(ctx, req)
	if err != nil {
		return err
	}

	return nil
}

// AccessInfo represents access credentials for a bucket, such as access keys or tokens.
type AccessInfo struct {
	AccountID   string
	AccessKeyID string
	SecretKey   string
	Buckets     []BucketInfo
}

// CreateBucketAccess creates access credentials for a bucket.
func (c *Client) CreateBucketAccess(ctx context.Context, userID string, buckets []string, params map[string]string) (*AccessInfo, error) {
	name, ok := params[ParamAccessSecretName]
	if !ok {
		return nil, errMissingParameter(ParamAccessSecretName)
	}

	namespace, ok := params[ParamAccessSecretNamespace]
	if !ok {
		return nil, errMissingParameter(ParamAccessSecretNamespace)
	}

	var errs error

	bucketInfos := make([]BucketInfo, 0, len(buckets))

	for _, bucket := range buckets {
		req := &s3.HeadBucketInput{
			Bucket: new(bucket),
		}

		resp, err := c.s3cli.HeadBucket(ctx, req)
		if err != nil {
			errs = errors.Join(errs, err)

			continue
		}

		bucketInfos = append(bucketInfos, BucketInfo{
			BucketName: bucket,
			Endpoint:   c.endpoint,
			Region:     c.resolveRegion(resp.BucketRegion),
		})

	}

	secret, err := c.kubeCli.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &AccessInfo{
		AccountID:   userID,
		AccessKeyID: string(secret.Data[AWS_ACCESS_KEY_ID]),
		SecretKey:   string(secret.Data[AWS_SECRET_ACCESS_KEY]),
		Buckets:     bucketInfos,
	}, nil
}

// DeleteBucketAccess removes access credentials for a bucket.
func (c *Client) DeleteBucketAccess(_ context.Context, _ string, _ []string, _ map[string]string) error {
	return nil
}

func GetS3Credentials() (creds map[string]string, err error) {
	var config *rest.Config

	creds = make(map[string]string)

	ns := os.Getenv(POD_NAMESPACE)
	if ns == "" {
		return creds, fmt.Errorf("%s not set", POD_NAMESPACE)
	}

	secretName := os.Getenv(S3_CREDENTIALS_SECRET_NAME)
	if secretName == "" {
		return creds, fmt.Errorf("%s not set", S3_CREDENTIALS_SECRET_NAME)
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
			return creds, err
		}
	}

	// 3. Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return creds, err
	}

	secret, err := clientset.CoreV1().Secrets(ns).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return creds, err
	}

	accessKey := string(secret.Data[AWS_ACCESS_KEY_ID])
	secretAccessKey := string(secret.Data[AWS_SECRET_ACCESS_KEY])
	endpoint := string(secret.Data[AWS_ENDPOINT_URL])
	if accessKey == "" {
		return creds, fmt.Errorf("%s missing in S3 secret", AWS_ACCESS_KEY_ID)
	}
	if secretAccessKey == "" {
		return creds, fmt.Errorf("%s missing in S3 secret", AWS_SECRET_ACCESS_KEY)
	}
	if endpoint == "" {
		return creds, fmt.Errorf("%s missing in S3 secret", AWS_ENDPOINT_URL)
	}
	creds[AWS_ACCESS_KEY_ID] = accessKey
	creds[AWS_SECRET_ACCESS_KEY] = secretAccessKey
	creds[AWS_ENDPOINT_URL] = endpoint
	creds[ParamAccessSecretName] = secretName
	creds[ParamAccessSecretNamespace] = ns
	creds[ParamAdminSecretName] = secretName
	creds[ParamAdminSecretNamespace] = ns
	return creds, nil
}
