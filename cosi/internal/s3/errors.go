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

// MissingParameterError represents an error indicating that a required parameter is missing.
type MissingParameterError struct {
	Parameter string
}

// Error impements error interface.
func (e MissingParameterError) Error() string {
	return "missing required parameter: " + e.Parameter
}

// Is implements errors.Is() interface.
func (e MissingParameterError) Is(target error) bool {
	err, ok := target.(*MissingParameterError)
	if !ok {
		return false
	}

	return err.Parameter == e.Parameter
}

func errMissingParameter(param string) error {
	return MissingParameterError{Parameter: param}
}

// BucketNotFoundError represents an error indicating that a bucket was not found.
type BucketNotFoundError struct {
	Bucket string
}

// Error impements error interface.
func (e BucketNotFoundError) Error() string {
	return "bucket not found: " + e.Bucket
}

// Is implements errors.Is() interface.
func (e BucketNotFoundError) Is(target error) bool {
	err, ok := target.(*BucketNotFoundError)
	if !ok {
		return false
	}

	return err.Bucket == e.Bucket
}

func errBucketNotFound(bucket string) error {
	return BucketNotFoundError{Bucket: bucket}
}
