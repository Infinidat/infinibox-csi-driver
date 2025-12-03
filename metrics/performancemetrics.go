package metric

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	FieldOps        = "ops"
	FieldLatencyNAS = "latency"
	FieldLatencySAN = "external_latency_wout_err"
	FieldThroughput = "throughput"
)

var (
	PerfIOPS = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxPerfIOPS,
		Help: "The ibox IOPs",
	}, []string{MetricIboxIP, MetricIboxHostname, MetricIboxProtocol})
	PerfThroughput = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxPerfThroughput,
		Help: "The ibox throughput",
	}, []string{MetricIboxIP, MetricIboxHostname, MetricIboxProtocol})
	PerfLatency = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxPerfLatency,
		Help: "The ibox latency",
	}, []string{MetricIboxIP, MetricIboxHostname, MetricIboxProtocol})
)

func RecordPerformanceMetrics(ctx context.Context, config *MetricsConfig) {
	slog.Log(ctx, common.LevelTrace, "performance metrics recording...")
	go func() {
		for {
			time.Sleep(config.GetDuration(MetricIboxPerfMetrics))

			for i := range config.Ibox {
				ibox := config.Ibox[i]
				slog.Log(ctx, common.LevelTrace, "performance metrics: creating collectors for..", "ibox", ibox.IboxHostname)
				nasID, sanID, err := createCollectors(ctx, ibox)
				if err != nil {
					slog.Error(err.Error())
					continue
				}

				time.Sleep(time.Second * 5) // this is necessary to give the ibox time to fire up the collectors

				slog.Log(ctx, common.LevelTrace, "performance metrics: get NAS collector data", "nas id", nasID, "san id", sanID)
				response, err := getCollectorData(ctx, nasID, ibox)
				if err != nil {
					slog.Error(err.Error())
					continue
				}
				slog.Log(ctx, common.LevelTrace, "performance metrics", "nas data", response)
				opsAverage, throughputAverage, latencyAverage := getCounterAverages(response.Result.Collectors[0].Fields, response.Result.Collectors[0].Data)
				slog.Log(ctx, common.LevelTrace, "performance metrics: nas metric averages", "opsaverage", opsAverage, "throughputaverage", throughputAverage, "latencyavg", latencyAverage)
				labels := prometheus.Labels{
					MetricIboxIP:       ibox.IboxIPAddress,
					MetricIboxHostname: ibox.IboxHostname,
					MetricIboxProtocol: "NAS"}

				PerfIOPS.With(labels).Set(float64(opsAverage))
				PerfThroughput.With(labels).Set(float64(throughputAverage))
				PerfLatency.With(labels).Set(float64(latencyAverage))

				sanResponse, err := getCollectorData(ctx, sanID, ibox)
				if err != nil {
					slog.Error(err.Error())
					continue
				}
				slog.Log(ctx, common.LevelTrace, "performance metrics", "san data", sanResponse)
				opsAverage, throughputAverage, latencyAverage = getCounterAverages(sanResponse.Result.Collectors[0].Fields, sanResponse.Result.Collectors[0].Data)
				slog.Log(ctx, common.LevelTrace, "performance metrics: san metric averages", "ops", opsAverage, "throughtput", throughputAverage, "latency", latencyAverage)

				err = deleteCollector(ctx, nasID, ibox)
				if err != nil {
					slog.Error(err.Error())
					continue
				}
				slog.Log(ctx, common.LevelTrace, "performance metrics: deleted NAS collector", "nas id", nasID)
				err = deleteCollector(ctx, sanID, ibox)
				if err != nil {
					slog.Error(err.Error())
					continue
				}
				slog.Log(ctx, common.LevelTrace, "performance metrics: deleted SAN collector", "san id", sanID)

				labels = prometheus.Labels{
					MetricIboxIP:       ibox.IboxIPAddress,
					MetricIboxHostname: ibox.IboxHostname,
					MetricIboxProtocol: "SAN"}

				PerfIOPS.With(labels).Set(float64(opsAverage))
				PerfThroughput.With(labels).Set(float64(throughputAverage))
				PerfLatency.With(labels).Set(float64(latencyAverage))
			}
		}
	}()
}

func getCollectorData(ctx context.Context, collectorID int64, ibox IboxCredentials) (*CollectorResponse, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/api/rest/metrics/collectors/data?collector_id=%d", ibox.IboxHostname, collectorID), http.NoBody)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	req.SetBasicAuth(ibox.IboxUsername, ibox.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}()

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	response := &CollectorResponse{}
	err = json.Unmarshal(responseData, response)
	if err != nil {
		return nil, err
	}

	// TODO proper check of error code/message goes here

	// curl -u "csitesting:csitestingisfun"
	// https://ibox1521.lab.wt.us.infinidat.com/api/rest/metrics/collectors/data?collector_id=35184372295290
	// --insecure
	return response, nil
}

func createCollectors(ctx context.Context, ibox IboxCredentials) (nasCollectorID int64, sanCollectorID int64, err error) {
	type Filters struct {
		ProtocolType string `json:"protocol_type"`
	}
	type RequestJSON struct {
		CollectedFields []string `json:"collected_fields"`
		Type            string   `json:"type"`
		Filters         Filters  `json:"filters"`
	}

	type CreateCollectorResponse struct {
		Result struct {
			ID      int64 `json:"id"`
			Filters struct {
				ProtocolType string `json:"protocol_type"`
			} `json:"filters"`
			FilterID        int64    `json:"filter_id"`
			CollectedFields []string `json:"collected_fields"`
			Type            string   `json:"type"`
		} `json:"result"`
		Error    any `json:"error"`
		Metadata struct {
			Ready bool `json:"ready"`
		} `json:"metadata"`
	}

	// curl -u "csitesting:csitestingisfun" \
	// -d '{"collected_fields": ["ops","throughput","external_latency_wout_err"],\
	// "type": "COUNTER","filters": {"protocol_type": "SAN"}}' \
	// -H "Content-Type: application/json" \
	// -X POST http://ibox1521.lab.wt.us.infinidat.com/api/rest/metrics/collectors --insecure
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}
	var req *http.Request

	filters := Filters{
		ProtocolType: "SAN",
	}
	params := RequestJSON{
		CollectedFields: []string{FieldOps, FieldThroughput, FieldLatencySAN},
		Type:            "COUNTER",
		Filters:         filters,
	}

	var jsonData []byte
	jsonData, err = json.Marshal(params)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}
	buff := bytes.NewBuffer(jsonData)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s/api/rest/metrics/collectors", ibox.IboxHostname), buff)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}

	req.SetBasicAuth(ibox.IboxUsername, ibox.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	var res *http.Response
	res, err = client.Do(req)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}()

	var sanResponseData []byte
	sanResponseData, err = io.ReadAll(res.Body)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}
	slog.Log(ctx, common.LevelTrace, "san collector create response", "san response", string(sanResponseData))

	sanresponse := &CreateCollectorResponse{}
	err = json.Unmarshal(sanResponseData, sanresponse)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}
	// TODO proper check of error code/message goes here

	// curl -u "csitesting:csitestingisfun" -d '{"collected_fields": ["ops","throughput","latency"],"type": "COUNTER","filters": {"protocol_type": "NAS"}}' -H "Content-Type: application/json" -X POST http://ibox1521.lab.wt.us.infinidat.com/api/rest/metrics/collectors --insecure
	filters = Filters{
		ProtocolType: "NAS",
	}
	params = RequestJSON{
		CollectedFields: []string{FieldOps, FieldThroughput, FieldLatencyNAS},
		Type:            "COUNTER",
		Filters:         filters,
	}

	jsonData, err = json.Marshal(params)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}
	buff = bytes.NewBuffer(jsonData)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s/api/rest/metrics/collectors", ibox.IboxHostname), buff)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}

	req.SetBasicAuth(ibox.IboxUsername, ibox.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err = client.Do(req)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}()

	var nasResponseData []byte
	nasResponseData, err = io.ReadAll(res.Body)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}

	slog.Log(ctx, common.LevelTrace, "nas collector post response", "nas response", string(nasResponseData))

	nasresponse := &CreateCollectorResponse{}
	err = json.Unmarshal(nasResponseData, nasresponse)
	if err != nil {
		slog.Error(err.Error())
		return nasCollectorID, sanCollectorID, err
	}
	// TODO proper check of error code/message goes here

	if nasresponse.Result.ID == 0 {
		return nasCollectorID, sanCollectorID, errors.New("nas collector not found")
	}
	if sanresponse.Result.ID == 0 {
		return nasCollectorID, sanCollectorID, errors.New("san collector not found")
	}
	nasCollectorID = nasresponse.Result.ID
	sanCollectorID = sanresponse.Result.ID
	return nasCollectorID, sanCollectorID, nil
}

func deleteCollector(ctx context.Context, collectorID int64, ibox IboxCredentials) error {
	// curl -u "csitesting:csitestingisfun" -X DELETE http://ibox1521.lab.wt.us.infinidat.com/api/rest/metrics/collectors/35184372295290   --insecure
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}
	var req *http.Request

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("https://%s/api/rest/metrics/collectors/%d", ibox.IboxHostname, collectorID), http.NoBody)
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	req.SetBasicAuth(ibox.IboxUsername, ibox.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	var res *http.Response
	res, err = client.Do(req)
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}()

	type DeleteCollectorResponse struct {
		Result struct {
			ID int64 `json:"id"`
		} `json:"result"`
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		Metadata struct {
			Ready bool `json:"ready"`
		} `json:"metadata"`
	}
	response, err := io.ReadAll(res.Body)
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	deleteresponse := &DeleteCollectorResponse{}
	err = json.Unmarshal(response, deleteresponse)
	if err != nil {
		slog.Error(err.Error())
		return err
	}
	slog.Log(ctx, common.LevelTrace, "delete collector response", "delete response", deleteresponse)

	// TODO proper check of error code/message goes here
	return nil
}

// get counter data from a collector
type CollectorResponse struct {
	Result struct {
		Collectors []struct {
			ID                       int64    `json:"id"`
			Fields                   []string `json:"fields"`
			Data                     [][]int  `json:"data"`
			CollectorType            string   `json:"collector_type"`
			IntervalMilliseconds     int      `json:"interval_milliseconds"`
			EndTimestampMilliseconds int64    `json:"end_timestamp_milliseconds"`
		} `json:"collectors"`
	} `json:"result"`
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
	Metadata struct {
		Ready bool `json:"ready"`
	} `json:"metadata"`
}

func getCounterAverages(fields []string, data [][]int) (opsAverage int, throughputAverage int, latencyAverage int) {
	var opsIndex, opsTotal, latencyIndex, latencyTotal, throughputIndex, throughputTotal int
	for index := range fields {
		switch fields[index] {
		case FieldOps:
			opsIndex = index
		case FieldLatencyNAS, FieldLatencySAN:
			latencyIndex = index
		case FieldThroughput:
			throughputIndex = index
		}
	}
	for _, sample := range data {
		ops := sample[opsIndex]
		opsTotal += ops
		latency := sample[latencyIndex]
		latencyTotal += latency
		throughput := sample[throughputIndex]
		throughputTotal += throughput
	}
	if len(data) > 0 {
		opsAverage = (opsTotal / len(data))
		latencyAverage = (latencyTotal / len(data))
		throughputAverage = (throughputTotal / len(data))
	}
	return opsAverage, throughputAverage, latencyAverage
}
