package metric

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/klog/v2"
)

const (
	FIELD_OPS         = "ops"
	FIELD_LATENCY_NAS = "latency"
	FIELD_LATENCY_SAN = "external_latency_wout_err"
	FIELD_THROUGHPUT  = "throughput"
)

var (
	MetricPerfIOPSGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_PERFORMANCE_IOPS,
		Help: "The ibox IOPs",
	}, []string{METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME, METRIC_IBOX_PROTOCOL})
	MetricPerfThroughputGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_PERFORMANCE_THROUGHPUT,
		Help: "The ibox throughput",
	}, []string{METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME, METRIC_IBOX_PROTOCOL})
	MetricPerfLatencyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_PERFORMANCE_LATENCY,
		Help: "The ibox latency",
	}, []string{METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME, METRIC_IBOX_PROTOCOL})
)

func RecordPerformanceMetrics(config *MetricsConfig) {
	klog.V(4).Infof("performance metrics recording...")
	go func() {
		for {
			time.Sleep(config.GetDuration(METRIC_IBOX_PERFORMANCE_METRICS))

			klog.V(4).Info("performance metrics: creating collectors...")
			nasID, sanID, err := createCollectors(config)
			if err != nil {
				klog.Error(err)
				continue
			}

			time.Sleep(time.Second * 5) // this is necessary to give the ibox time to fire up the collectors

			klog.V(4).Info("performance metrics: get NAS collector data nasID %d sanID %d", nasID, sanID)
			nasResponse, err := getCollectorData(nasID, config)
			if err != nil {
				klog.Error(err)
				continue
			}
			klog.V(4).Infof("performance metrics: nas data %+v\n", nasResponse)
			opsAverage, throughputAverage, latencyAverage := getCounterAverages(nasResponse.Result.Collectors[0].Fields, nasResponse.Result.Collectors[0].Data)
			klog.V(4).Infof("performance metrics: nas metric averages ops %d throughput %d latency %d\n", opsAverage, throughputAverage, latencyAverage)
			if err == nil {
				labels := prometheus.Labels{
					METRIC_IBOX_IP:       config.IboxIpAddress,
					METRIC_IBOX_HOSTNAME: config.IboxHostname,
					METRIC_IBOX_PROTOCOL: "NAS"}

				MetricPerfIOPSGauge.With(labels).Set(float64(opsAverage))
				MetricPerfThroughputGauge.With(labels).Set(float64(throughputAverage))
				MetricPerfLatencyGauge.With(labels).Set(float64(latencyAverage))
			}

			sanResponse, err := getCollectorData(sanID, config)
			if err != nil {
				klog.Error(err)
				continue
			}
			klog.V(4).Infof("performance metrics: san data %+v\n", sanResponse)
			opsAverage, throughputAverage, latencyAverage = getCounterAverages(sanResponse.Result.Collectors[0].Fields, sanResponse.Result.Collectors[0].Data)
			klog.V(4).Infof("performance metrics: san metric averages ops %d throughput %d latency %d\n", opsAverage, throughputAverage, latencyAverage)

			err = deleteCollector(nasID, config)
			if err != nil {
				klog.Error(err)
				continue
			}
			klog.V(4).Infof("performance metrics: deleted NAS collector %d\n", nasID)
			err = deleteCollector(sanID, config)
			if err != nil {
				klog.Error(err)
				continue
			}
			klog.V(4).Infof("performance metrics: deleted SAN collector %d\n", sanID)

			labels := prometheus.Labels{
				METRIC_IBOX_IP:       config.IboxIpAddress,
				METRIC_IBOX_HOSTNAME: config.IboxHostname,
				METRIC_IBOX_PROTOCOL: "SAN"}

			MetricPerfIOPSGauge.With(labels).Set(float64(opsAverage))
			MetricPerfThroughputGauge.With(labels).Set(float64(throughputAverage))
			MetricPerfLatencyGauge.With(labels).Set(float64(latencyAverage))

		}
	}()
}

func getCollectorData(collectorID int64, config *MetricsConfig) (*CollectorResponse, error) {
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

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/api/rest/metrics/collectors/data?collector_id=%d", config.IboxHostname, collectorID), http.NoBody)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	req.SetBasicAuth(config.IboxUsername, config.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	defer res.Body.Close()

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	response := &CollectorResponse{}
	err = json.Unmarshal(responseData, response)
	if err != nil {
		return nil, err
	}

	//TODO proper check of error code/message goes here

	// curl -u "csitesting:csitestingisfun" https://ibox1521.lab.wt.us.infinidat.com/api/rest/metrics/collectors/data?collector_id=35184372295290 --insecure
	return response, nil
}

func createCollectors(config *MetricsConfig) (NAScollectorID int64, SANcollectorID int64, err error) {

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
		CollectedFields: []string{FIELD_OPS, FIELD_THROUGHPUT, FIELD_LATENCY_SAN},
		Type:            "COUNTER",
		Filters:         filters,
	}

	var jsonData []byte
	jsonData, err = json.Marshal(params)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}
	buff := bytes.NewBuffer(jsonData)
	req, err = http.NewRequest(http.MethodPost, fmt.Sprintf("https://%s/api/rest/metrics/collectors", config.IboxHostname), buff)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}

	req.SetBasicAuth(config.IboxUsername, config.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	var res *http.Response
	res, err = client.Do(req)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}

	defer res.Body.Close()

	var sanResponseData []byte
	sanResponseData, err = io.ReadAll(res.Body)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}
	klog.V(4).Infof("san collector create response %s\n", string(sanResponseData))

	sanresponse := &CreateCollectorResponse{}
	err = json.Unmarshal(sanResponseData, sanresponse)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}
	//TODO proper check of error code/message goes here

	// curl -u "csitesting:csitestingisfun" -d '{"collected_fields": ["ops","throughput","latency"],"type": "COUNTER","filters": {"protocol_type": "NAS"}}' -H "Content-Type: application/json" -X POST http://ibox1521.lab.wt.us.infinidat.com/api/rest/metrics/collectors --insecure
	filters = Filters{
		ProtocolType: "NAS",
	}
	params = RequestJSON{
		CollectedFields: []string{FIELD_OPS, FIELD_THROUGHPUT, FIELD_LATENCY_NAS},
		Type:            "COUNTER",
		Filters:         filters,
	}

	jsonData, err = json.Marshal(params)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}
	buff = bytes.NewBuffer(jsonData)
	req, err = http.NewRequest(http.MethodPost, fmt.Sprintf("https://%s/api/rest/metrics/collectors", config.IboxHostname), buff)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}

	req.SetBasicAuth(config.IboxUsername, config.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err = client.Do(req)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}

	defer res.Body.Close()

	var nasResponseData []byte
	nasResponseData, err = io.ReadAll(res.Body)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}

	klog.V(4).Infof("nas collector post response %s\n", string(nasResponseData))

	nasresponse := &CreateCollectorResponse{}
	err = json.Unmarshal(nasResponseData, nasresponse)
	if err != nil {
		klog.Error(err)
		return NAScollectorID, SANcollectorID, err
	}
	//TODO proper check of error code/message goes here

	if nasresponse.Result.ID == 0 {
		return NAScollectorID, SANcollectorID, errors.New("nas collector not found")
	}
	if sanresponse.Result.ID == 0 {
		return NAScollectorID, SANcollectorID, errors.New("san collector not found")

	}
	NAScollectorID = nasresponse.Result.ID
	SANcollectorID = sanresponse.Result.ID
	return NAScollectorID, SANcollectorID, nil
}
func deleteCollector(collectorID int64, config *MetricsConfig) error {
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

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("https://%s/api/rest/metrics/collectors/%d", config.IboxHostname, collectorID), http.NoBody)
	if err != nil {
		klog.Error(err)
		return err
	}

	req.SetBasicAuth(config.IboxUsername, config.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	var res *http.Response
	res, err = client.Do(req)
	if err != nil {
		klog.Error(err)
		return err
	}

	defer res.Body.Close()

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
		klog.Error(err)
		return err
	}

	deleteresponse := &DeleteCollectorResponse{}
	err = json.Unmarshal(response, deleteresponse)
	if err != nil {
		klog.Error(err)
		return err
	}
	klog.V(4).Infof("delete collector response %+v\n", deleteresponse)

	//TODO proper check of error code/message goes here
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

	var opsIndex int
	var opsTotal int
	var latencyIndex int
	var latencyTotal int
	var throughputIndex int
	var throughputTotal int
	for i := 0; i < len(fields); i++ {
		if fields[i] == FIELD_OPS {
			opsIndex = i
		} else if fields[i] == FIELD_LATENCY_NAS || fields[i] == FIELD_LATENCY_SAN {
			latencyIndex = i
		} else if fields[i] == FIELD_THROUGHPUT {
			throughputIndex = i
		}
	}
	for sample := 0; sample < len(data); sample++ {
		s := data[sample]
		ops := s[opsIndex]
		opsTotal = opsTotal + ops
		latency := s[latencyIndex]
		latencyTotal = latencyTotal + latency
		throughput := s[throughputIndex]
		throughputTotal = throughputTotal + throughput
	}
	if len(data) > 0 {
		opsAverage = (opsTotal / len(data))
		latencyAverage = (latencyTotal / len(data))
		throughputAverage = (throughputTotal / len(data))
	}
	return opsAverage, throughputAverage, latencyAverage
}
