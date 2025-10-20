/*
Copyright 2025 Infinidat
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
package nfs

import (
	"strings"
)

const (
	NFSv3Port = "2049"
	NFSv4Port = "12049"
)

func GetNFSVersionPort(mountOptions []string) (version, port string) {
	// we will default to nfs v3
	version = "3"
	port = NFSv3Port

	for _, opt := range mountOptions {
		if strings.Contains(opt, "vers") {
			parts := strings.Split(opt, "=")
			if len(parts) == 2 {
				version = parts[1]
			}
		}
		if strings.Contains(opt, "port") {
			parts := strings.Split(opt, "=")
			if len(parts) == 2 {
				port = parts[1]
			}
		}
	}
	if version == "4" || version == "4.1" {
		port = NFSv4Port
	}
	return version, port
}
