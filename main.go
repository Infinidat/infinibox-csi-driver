/*
Copyright 2022 Infinidat
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
package main

import (
	"infinibox-csi-driver/log"
	"infinibox-csi-driver/service"
	"os"
)

var version string
var compileDate string
var gitHash string

// starting method of CSI-Driver
func main() {

	log.CheckForLogLevelOverride()

	// this call effectively initializes the logging system based on environment variables
	// and default configurations.
	zlog := log.Get()

	zlog.Info().Msg("Infinidat CSI Driver is Starting")
	zlog.Info().Msgf("Version: %s", version)
	zlog.Info().Msgf("Compile date: %s", compileDate)
	zlog.Info().Msgf("Compile git hash: %s", gitHash)
	zlog.Info().Msgf("Log level: %s", os.Getenv("APP_LOG_LEVEL"))

	log.SetupKlog()

	nodeIP := os.Getenv("NODE_IP")
	if nodeIP == "" {
		zlog.Error().Msg("NODE_IP not set")
		os.Exit(1)
	}
	driverName := os.Getenv("CSI_DRIVER_NAME")
	if driverName == "" {
		zlog.Error().Msg("CSI_DRIVER_NAME not set")
		os.Exit(1)
	}
	csiEndpoint := os.Getenv("CSI_ENDPOINT")
	if csiEndpoint == "" {
		zlog.Error().Msg("CSI_ENDPOINT not set")
		os.Exit(1)
	}
	if version == "" {
		zlog.Error().Msg("version not set")
		os.Exit(1)
	}

	zlog.Info().Msgf("NodeIP: %s", nodeIP)
	zlog.Info().Msgf("DriverName: %s", driverName)
	zlog.Info().Msgf("Endpoint: %s", csiEndpoint)
	zlog.Info().Msgf("Version: %s", version)

	driverOptions := service.DriverOptions{
		NodeID:     nodeIP,
		DriverName: driverName,
		Endpoint:   csiEndpoint,
		Version:    version,
	}
	d := service.NewDriver(&driverOptions)
	d.Run(false)
}
