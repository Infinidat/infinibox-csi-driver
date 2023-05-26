package main

import (
	"flag"
	"fmt"
	metric "infinibox-csi-driver/metrics"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

var version string
var compileDate string
var gitHash string

func main() {

	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("stderrthreshold", "WARNING")

	var verbosity string
	appLogLevel := os.Getenv("APP_LOG_LEVEL")
	switch appLogLevel {
	case "quiet":
		verbosity = "1"
	case "info":
		verbosity = "2"
	case "extended":
		verbosity = "3"
	case "debug":
		verbosity = "4"
	case "trace":
		verbosity = "5"
	default:
		verbosity = "5"
	}
	_ = flag.Set("v", verbosity)
	flag.Parse()

	klog.V(2).Infof("infinidat CSI metrics starting")
	klog.V(2).Infof("version: %s", version)
	klog.V(2).Infof("compile date: %s", compileDate)
	klog.V(2).Infof("compile git hash: %s", gitHash)
	klog.Flush()

	config, err := metric.NewConfig()
	if err != nil {
		klog.Errorf("could not read metrics config file", err.Error())
		os.Exit(1)
	}
	klog.V(4).Infof("config is %+v", config)

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
