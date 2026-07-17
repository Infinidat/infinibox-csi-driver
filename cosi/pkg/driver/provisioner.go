package driver

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
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/infinidat/infinibox-csi-driver/cosi/internal/s3"
	"k8s.io/klog/v2"
	cosi "sigs.k8s.io/container-object-storage-interface/proto"
)

var ErrBucketNotFound = errors.New("bucket not found")

// ProvisionerServer implements the COSI driver server interface.
type ProvisionerServer struct {
	S3Credentials map[string]string
	cosi.UnimplementedProvisionerServer

	DynamicClient func(ctx context.Context, params map[string]string) (s3.DynamicClient, error)
}

// DriverCreateBucket creates a bucket if it does not already exist.
// If the bucket exists and the parameters match, it returns success without error.
// If the bucket exists but the parameters differ, it returns a conflict error.
func (s *ProvisionerServer) DriverCreateBucket(
	ctx context.Context,
	req *cosi.DriverCreateBucketRequest,
) (*cosi.DriverCreateBucketResponse, error) {
	bucketName := req.GetName()
	parameters := req.GetParameters()

	slog.Info("DriverCreateBucket called", "bucket", bucketName, "params", parameters)

	s3cli, err := s.DynamicClient(ctx, parameters)
	if err != nil {
		if mpErr, ok := errors.AsType[s3.MissingParameterError](err); ok {
			slog.Error("Failed to initialize S3 client due to missing parameter", "bucket", bucketName, "error", err)
			return nil, status.Error(codes.InvalidArgument, "Failed to initialize S3 client due to missing parameter: \""+mpErr.Parameter+"\"")
		}

		slog.Error("Failed to initialize S3 client", "bucket", bucketName, "parameters", parameters, "error", err)
		return nil, status.Error(codes.Internal, "Failed to initialize S3 client")
	}

	klog.InfoS("before BucketInfo call", "bucketName", bucketName)

	bucketInfo, err := s3cli.BucketInfo(ctx, bucketName)
	if err != nil {
		if _, ok := errors.AsType[s3.BucketNotFoundError](err); !ok {
			slog.Error("Failed to check bucket existence", "bucket", bucketName, "parameters", parameters, "error", err)
			return nil, status.Error(codes.Internal, "Failed to check bucket existence")
		}
	}

	if bucketInfo == nil {
		bucketInfo, err = s3cli.CreateBucket(ctx, bucketName, parameters)
		if err != nil {
			if mpErr, ok := errors.AsType[s3.MissingParameterError](err); ok {
				slog.Error("Failed to create bucket due to missing parameter", "bucket", bucketName, "error", err)
				return nil, status.Error(codes.InvalidArgument, "Failed to create bucket due to missing parameter: \""+mpErr.Parameter+"\"")
			}

			slog.Error("Failed to create bucket", "bucket", bucketName, "error", err)
			return nil, status.Error(codes.Internal, "Failed to create bucket")
		}
	}

	slog.Info("Bucket successfully created", "bucket", bucketName)
	return &cosi.DriverCreateBucketResponse{
		BucketId: bucketInfo.BucketName,
		BucketInfo: &cosi.Protocol{
			Type: &cosi.Protocol_S3{
				S3: &cosi.S3{
					Region: bucketInfo.Region,
				},
			},
		},
	}, nil
}

// DriverDeleteBucket deletes a bucket if it exists. If the bucket does not exist, it returns success.
func (s *ProvisionerServer) DriverDeleteBucket(
	ctx context.Context,
	req *cosi.DriverDeleteBucketRequest,
) (*cosi.DriverDeleteBucketResponse, error) {
	bucketId := req.GetBucketId()

	if len(s.S3Credentials) == 0 {
		slog.Error("DriverDeleteBucket - S3Credentials are missing", "bucket", bucketId)
		return nil, status.Error(codes.InvalidArgument, "missing S3Credentials")
	}

	slog.Info("DriverDeleteBucket called", "bucketId", bucketId, "S3Credentials", s.S3Credentials)

	s3cli, err := s.DynamicClient(ctx, s.S3Credentials)
	if err != nil {
		if mpErr, ok := errors.AsType[s3.MissingParameterError](err); ok {
			slog.Error("Failed to initialize S3 client due to missing parameter", "bucket", bucketId, "error", err)
			return nil, status.Error(codes.InvalidArgument, "Failed to initialize S3 client due to missing parameter: \""+mpErr.Parameter+"\"")
		}

		slog.Error("Failed to initialize S3 client", "bucket", bucketId, "error", err)
		return nil, status.Error(codes.Internal, "Failed to initialize S3 client")
	}

	if err := s3cli.DeleteBucket(ctx, bucketId, s.S3Credentials); err != nil {
		if mpErr, ok := errors.AsType[s3.MissingParameterError](err); ok {
			slog.Error("Failed to delete bucket due to missing parameter", "bucket", bucketId, "error", err)
			return nil, status.Error(codes.InvalidArgument, "Failed to delete bucket due to missing parameter: \""+mpErr.Parameter+"\"")
		}

		slog.Error("Failed to delete bucket", "bucket", bucketId, "error", err)
		return nil, status.Error(codes.Internal, "Failed to delete bucket")
	}

	slog.Info("Bucket successfully deleted", "bucket", bucketId)
	return &cosi.DriverDeleteBucketResponse{}, nil
}

// DriverGrantBucketAccess grants access to a bucket. It creates an access account for the given bucket and user.
//
// Return values:
//   - nil: Access successfully granted.
//   - error: Internal error requiring retries.
func (s *ProvisionerServer) DriverGrantBucketAccess(
	ctx context.Context,
	req *cosi.DriverGrantBucketAccessRequest,
) (*cosi.DriverGrantBucketAccessResponse, error) {
	//name := req.GetAccountName()
	name := req.GetName()
	parameters := req.GetParameters()

	slog.Info("DriverGrantBucketAccess called", "name", name, "params", parameters)

	s3cli, err := s.DynamicClient(ctx, parameters)
	if err != nil {
		if mpErr, ok := errors.AsType[s3.MissingParameterError](err); ok {
			slog.Error("Failed to initialize S3 client due to missing parameter", "account", name, "error", err)
			return nil, status.Error(codes.InvalidArgument, "Failed to initialize S3 client due to missing parameter: \""+mpErr.Parameter+"\"")
		}

		slog.Error("Failed to initialize S3 client", "account", name, "error", err)
		return nil, status.Error(codes.Internal, "Failed to initialize S3 client")
	}

	_, err = s3cli.BucketInfo(ctx, req.BucketId)
	if err != nil {
		slog.Error("Failed to check bucket existence", "bucket", req.BucketId, "account", name, "error", err)
		return nil, status.Error(codes.Internal, "Failed to check bucket existence")
	}

	bucketIds := make([]string, 0)
	bucketIds = append(bucketIds, req.GetBucketId())

	accessInfo, err := s3cli.CreateBucketAccess(ctx, name, bucketIds, parameters)
	if err != nil {
		if mpErr, ok := errors.AsType[s3.MissingParameterError](err); ok {
			slog.Error("Failed to create bucket access due to missing parameter", "account", name, "error", err)
			return nil, status.Error(codes.InvalidArgument, "Failed to create bucket access due to missing parameter: \""+mpErr.Parameter+"\"")
		}

		slog.Error("Failed to create bucket access", "buckets", bucketIds, "account", name, "error", err)
		return nil, status.Error(codes.Internal, "Failed to create bucket access")
	}

	slog.Info("Bucket access successfully granted", "name", "")

	secrets := make(map[string]string)

	credentialDetails := &cosi.CredentialDetails{
		Secrets: secrets,
	}
	credentialsMap := make(map[string]*cosi.CredentialDetails)
	credentialsMap["somecredential"] = credentialDetails

	return &cosi.DriverGrantBucketAccessResponse{
		AccountId:   accessInfo.AccountID,
		Credentials: credentialsMap,
	}, nil
}

// DriverRevokeBucketAccess revokes access to a bucket for a specific account.
// If the access does not exist, it returns success.
func (s *ProvisionerServer) DriverRevokeBucketAccess(
	ctx context.Context,
	req *cosi.DriverRevokeBucketAccessRequest,
) (*cosi.DriverRevokeBucketAccessResponse, error) {
	accountId := req.GetAccountId()

	if len(s.S3Credentials) == 0 {
		slog.Error("DriverRevokeBucketAccess - S3Credentials are missing", "accountId", accountId)
		return nil, status.Error(codes.InvalidArgument, "missing S3Credentials")
	}

	slog.Info("DriverRevokeBucketAccess called", "accountId", accountId, "credentials", s.S3Credentials)

	s3cli, err := s.DynamicClient(ctx, s.S3Credentials)
	if err != nil {
		if mpErr, ok := errors.AsType[s3.MissingParameterError](err); ok {
			slog.Error("Failed to initialize S3 client due to missing parameter", "account", accountId, "error", err)
			return nil, status.Error(codes.InvalidArgument, "Failed to initialize S3 client due to missing parameter: \""+mpErr.Parameter+"\"")
		}

		slog.Error("Failed to initialize S3 client", "account", accountId, "error", err)
		return nil, status.Error(codes.Internal, "Failed to initialize S3 client")
	}

	bucketIds := make([]string, 0, 1)

	bucketIds = append(bucketIds, req.BucketId)

	if err := s3cli.DeleteBucketAccess(ctx, accountId, bucketIds, s.S3Credentials); err != nil {
		if mpErr, ok := errors.AsType[s3.MissingParameterError](err); ok {
			slog.Error("Failed to revoke bucket access due to missing parameter", "account", accountId, "error", err)
			return nil, status.Error(codes.InvalidArgument, "Failed to revoke bucket access due to missing parameter: \""+mpErr.Parameter+"\"")
		}

		slog.Error("Failed to revoke bucket access", "buckets", bucketIds, "account", accountId, "error", err)
		return nil, status.Error(codes.Internal, "Failed to revoke bucket access")
	}

	slog.Info("Bucket access successfully revoked", "buckets", bucketIds, "account", accountId)
	return &cosi.DriverRevokeBucketAccessResponse{}, nil
}
