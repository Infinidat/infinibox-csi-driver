// Package s3 provides stub implementations of the S3 client interfaces for testing purposes.
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

	"github.com/infinidat/infinibox-csi-driver/cosi/internal/s3"
)

// StubClient is an in-memory implementation of s3.DynamicClient for use in tests.
// It stores buckets and accesses in plain maps, and exposes per-method error fields
// so individual operations can be made to fail without replacing the whole client.
type StubClient struct {
	// Buckets is the in-memory bucket store keyed by bucket name.
	Buckets map[string]*s3.BucketInfo
	// Accesses is the in-memory access store keyed by account ID.
	Accesses map[string]*s3.AccessInfo

	// Per-method error overrides. When non-nil the corresponding method returns
	// the error immediately, bypassing the in-memory store.
	BucketInfoErr      error
	CreateBucketErr    error
	DeleteBucketErr    error
	CreateBucketAccErr error
	DeleteBucketAccErr error
}

var _ s3.DynamicClient = (*StubClient)(nil)

func (m *StubClient) BucketInfo(_ context.Context, bucket string) (*s3.BucketInfo, error) {
	if m.BucketInfoErr != nil {
		return nil, m.BucketInfoErr
	}

	b, ok := m.Buckets[bucket]
	if !ok {
		return nil, s3.BucketNotFoundError{Bucket: bucket}
	}

	return b, nil
}

func (m *StubClient) CreateBucket(_ context.Context, bucket string, _ map[string]string) (*s3.BucketInfo, error) {
	if m.CreateBucketErr != nil {
		return nil, m.CreateBucketErr
	}

	if m.Buckets == nil {
		m.Buckets = make(map[string]*s3.BucketInfo)
	}

	if _, ok := m.Buckets[bucket]; ok {
		return m.Buckets[bucket], nil
	}

	m.Buckets[bucket] = &s3.BucketInfo{BucketName: bucket}

	return m.Buckets[bucket], nil
}

func (m *StubClient) DeleteBucket(_ context.Context, bucket string, _ map[string]string) error {
	if m.DeleteBucketErr != nil {
		return m.DeleteBucketErr
	}

	if m.Buckets == nil {
		m.Buckets = make(map[string]*s3.BucketInfo)
	}

	delete(m.Buckets, bucket)

	return nil
}

func (m *StubClient) CreateBucketAccess(_ context.Context, userID string, buckets []string, _ map[string]string) (*s3.AccessInfo, error) {
	if m.CreateBucketAccErr != nil {
		return nil, m.CreateBucketAccErr
	}

	if m.Accesses == nil {
		m.Accesses = make(map[string]*s3.AccessInfo)
	}

	if m.Buckets == nil {
		m.Buckets = make(map[string]*s3.BucketInfo)
	}

	bucketInfos := make([]s3.BucketInfo, 0, len(buckets))

	for _, bucket := range buckets {
		b, ok := m.Buckets[bucket]
		if !ok {
			return nil, s3.BucketNotFoundError{Bucket: bucket}
		}

		bucketInfos = append(bucketInfos, *b)
	}

	accessInfo := &s3.AccessInfo{
		AccountID: userID,
		Buckets:   bucketInfos,
	}

	m.Accesses[userID] = accessInfo

	return accessInfo, nil
}

func (m *StubClient) DeleteBucketAccess(_ context.Context, accessID string, _ []string, _ map[string]string) error {
	if m.DeleteBucketAccErr != nil {
		return m.DeleteBucketAccErr
	}

	if m.Accesses == nil {
		m.Accesses = make(map[string]*s3.AccessInfo)
	}

	delete(m.Accesses, accessID)

	return nil
}
