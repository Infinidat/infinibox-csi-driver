package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	metric "github.com/infinidat/infinibox-csi-driver/metrics"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var version string
var compileDate string
var gitHash string

const (
	CertFilePath = "/tmp/tls.crt"
	KeyFilePath  = "/tmp/tls.key"
)

func main() {
	logger := common.SetupSlog(true)

	logger.Info("infinidat CSI metrics starting", "version", version, "compile date", compileDate, "compile git hash", gitHash)

	// Get a k8s go client for in-cluster use
	client, err := clientgo.BuildClient()
	if err != nil {
		logger.Error("error", "getting client-go connection", err.Error())
		os.Exit(1)
	}

	namespace := os.Getenv("POD_NAMESPACE")
	logger.Info("env", "POD_NAMESPACE", namespace)
	if namespace == "" {
		slog.Error("env var POD_NAMESPACE was not set, defaulting to infinidat-csi namespace")
		namespace = "infinidat-csi"
	}

	ctx := context.Background()

	var secrets []map[string]string
	secrets, err = client.GetSecrets(ctx, namespace)
	if err != nil {
		slog.Error("error ", "getting secrets", err.Error())
	}
	config, err := metric.NewConfig(secrets)
	if err != nil {
		slog.Error("error", "could not read metrics config file", err.Error())
		os.Exit(1)
	}

	for index := range config.Ibox {
		tmp := config.Ibox[index]
		slog.Info("config", "ibox hostname", tmp.IboxHostname, "username", tmp.IboxUsername)
	}

	metric.RecordPVMetrics(ctx, config)
	metric.RecordPerformanceMetrics(ctx, config)
	metric.RecordPoolMetrics(ctx, config)
	metric.RecordSystemHealthMetrics(ctx, config)

	http.Handle("/", &home{})
	http.Handle("/metrics", promhttp.Handler())
	// _ = http.ListenAndServe(":"+*metric.PortFlag, nil)
	// load tls certificates
	tlsListenEnvVar := os.Getenv("TLS_LISTEN")
	slog.Info("env", "TLS_LISTEN", tlsListenEnvVar)
	tlsEnabled := false
	if tlsListenEnvVar != "" {
		tlsEnabled, err = strconv.ParseBool(tlsListenEnvVar)
		if err != nil {
			slog.Error("env var TLS_LISTEN was set, but value was not a boolean")
			os.Exit(1)
		}
	}
	slog.Info("value", "tlsEnabled", tlsEnabled)
	var tlsConfig *tls.Config

	if tlsEnabled {
		serverTLSCert, err := tls.LoadX509KeyPair(CertFilePath, KeyFilePath)
		if err != nil {
			slog.Error("error", "loading certificate and key file", err)
			os.Exit(1)
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{serverTLSCert},
		}
	}

	server := &http.Server{
		TLSConfig: tlsConfig,
		Addr:      ":" + *metric.PortFlag,
		// ReadHeaderTimeout is the amount of time allowed to read
		// request headers. The connection's read deadline is reset
		// after reading the headers and the Handler can decide what
		// is considered too slow for the body. If ReadHeaderTimeout
		// is zero, the value of ReadTimeout is used. If both are
		// zero, there is no timeout.
		ReadHeaderTimeout: 15 * time.Second,

		// ReadTimeout is the maximum duration for reading the entire
		// request, including the body. A zero or negative value means
		// there will be no timeout.
		//
		// Because ReadTimeout does not let Handlers make per-request
		// decisions on each request body's acceptable deadline or
		// upload rate, most users will prefer to use
		// ReadHeaderTimeout. It is valid to use them both.
		ReadTimeout: 15 * time.Second,

		// WriteTimeout is the maximum duration before timing out
		// writes of the response. It is reset whenever a new
		// request's header is read. Like ReadTimeout, it does not
		// let Handlers make decisions on a per-request basis.
		// A zero or negative value means there will be no timeout.
		WriteTimeout: 10 * time.Second,

		// IdleTimeout is the maximum amount of time to wait for the
		// next request when keep-alives are enabled. If IdleTimeout
		// is zero, the value of ReadTimeout is used. If both are
		// zero, there is no timeout.
		IdleTimeout: 30 * time.Second,
	}

	if tlsEnabled {
		if err := server.ListenAndServeTLS("", ""); err != nil {
			slog.Error("fatal error", "on srv.ListenAndServeTLS", err.Error())
			os.Exit(1)
		}
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("fatal error", "on srv.ListenAndServe", err.Error())
		os.Exit(1)
	}
}

type home struct{}

func (h *home) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
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

	bytesWritten, err := fmt.Fprintf(responseWriter, "%s", msg)
	if err != nil {
		slog.Error("error", "in ServeHTTP - error", err.Error(), "bytes written", bytesWritten)
	}
}
