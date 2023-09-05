package main

import (
	"fmt"
	"infinibox-csi-driver/api/clientgo"
	metric "infinibox-csi-driver/metrics"
	"net/http"
	"os"

	"infinibox-csi-driver/log"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var version string
var compileDate string
var gitHash string

func main() {

	var zlog = log.Get() // grab the logger for package use

	zlog.Info().Msgf("infinidat CSI metrics starting")
	zlog.Info().Msgf("version: %s", version)
	zlog.Info().Msgf("compile date: %s", compileDate)
	zlog.Info().Msgf("compile git hash: %s", gitHash)

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		zlog.Error().Msgf("error getting client-go connection %s", err.Error())
		os.Exit(1)
	}

	ns := os.Getenv("POD_NAMESPACE")
	zlog.Info().Msgf("POD_NAMESPACE=%s", ns)
	if ns == "" {
		zlog.Error().Msg("env var POD_NAMESPACE was not set, defaulting to infinidat-csi namespace")
		ns = "infinidat-csi"
	}

	var secrets []map[string]string
	secrets, err = cl.GetSecrets(ns)
	if err != nil {
		zlog.Error().Msgf("error getting secrets: %s", err.Error())
	}
	config, err := metric.NewConfig(secrets)
	if err != nil {
		zlog.Error().Msgf("could not read metrics config file: %s", err.Error())
		os.Exit(1)
	}

	for i := 0; i < len(config.Ibox); i++ {
		tmp := config.Ibox[i]
		zlog.Info().Msgf("config ibox hostname: %s username: %s", tmp.IboxHostname, tmp.IboxUsername)
	}

	metric.RecordPVMetrics(config)
	metric.RecordPerformanceMetrics(config)
	metric.RecordPoolMetrics(config)
	metric.RecordSystemHealthMetrics(config)

	http.Handle("/", &home{})
	http.Handle("/metrics", promhttp.Handler())
	_ = http.ListenAndServe(":"+*metric.PortFlag, nil)
}

type home struct{}

func (h *home) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	msg := `<html>
<body>
<h1>Infinidat CSI Driver Metrics Exporter</h1>
<table>
    <thead>
        <tr>
        <td>Type</td>
        <td>Endpoint</td>
        <td>GET parameters</td>
        <td>Description</td>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td>Full metrics</td>
            <td><a href="/metrics">/metrics</a></td>
            <td></td>
            <td>All metrics.</td>
        </tr>
	</tbody>
</table>
</body>
</html>`

	fmt.Fprintf(w, "%s", msg)
}
