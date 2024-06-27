package metric

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	jsonEndpoint = "http://" + os.Getenv("METRIC_WEB_ADDR") + "/debug/vars"
	metrics      = make(map[string]prometheus.Collector)
)

func sanitizeMetricName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, ":", "_"), ".", "_")
}

func fetchJSONData(url string) (map[string]interface{}, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&data)
	return data, err
}

func updateMetrics(data map[string]interface{}) {
	for key, value := range data {
		metricName := sanitizeMetricName(key)
		switch v := value.(type) {
		case float64:
			if _, exists := metrics[metricName]; !exists {
				gauge := prometheus.NewGauge(prometheus.GaugeOpts{
					Name: metricName,
					Help: "Metric for " + metricName,
				})
				prometheus.MustRegister(gauge)
				metrics[metricName] = gauge
				log.Printf("Created new Gauge metric: %s", metricName)
			}
			metrics[metricName].(prometheus.Gauge).Set(v)
		case map[string]interface{}:
			if strings.HasPrefix(key, "t_succ") {
				if _, exists := metrics[metricName]; !exists {
					histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
						Name:    metricName,
						Help:    "Metric for " + metricName,
						Buckets: prometheus.LinearBuckets(0.001, 0.001, 3),
					})
					prometheus.MustRegister(histogram)
					metrics[metricName] = histogram
					log.Printf("Created new Histogram metric: %s", metricName)
				}
				for quantile, quantileValue := range v {
					if qv, ok := quantileValue.(float64); ok {
						switch quantile {
						case "p50", "p90", "p99":
							metrics[metricName].(prometheus.Histogram).Observe(qv)
						}
					}
				}
			}
		}
	}
}

func fetchJSONMetric() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := fetchJSONData(jsonEndpoint)
		if err != nil {
			log.Printf("Error fetching JSON data: %v", err)
			http.Error(w, "Failed to fetch data", http.StatusInternalServerError)
			return
		}
		updateMetrics(data)

		promhttp.Handler().ServeHTTP(w, r)
	})
}

func init() {
	if os.Getenv("ENABLE_PROMETHEUS_METRICS") == "true" {
		http.Handle("/metrics", fetchJSONMetric())
	}
}
