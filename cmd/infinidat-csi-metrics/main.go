package main

import (
	"fmt"
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

	config, err := metric.NewConfig()
	if err != nil {
		zlog.Error().Msgf("could not read metrics config file: %s", err.Error())
		os.Exit(1)
	}
	zlog.Info().Msgf("config is %+v", config)

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
