package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// Define the structure of the mock data as per the provided JSON format
type MeasuredUsage struct {
	MetricID              string  `json:"metricId"`
	Value                 float64 `json:"value"`
	MetricType            string  `json:"metricType"`
	MetricAggregationType string  `json:"metricAggregationType"`
}

type MarketplaceReportData struct {
	EventID                        string          `json:"eventId"`
	Start                          int64           `json:"start"`
	End                            int64           `json:"end"`
	Source                         string          `json:"source"`
	SourceSaas                     string          `json:"sourceSaas"`
	LicensePartNumber              string          `json:"licensePartNumber"`
	ProductID                      string          `json:"productId"`
	ProductName                    string          `json:"productName"`
	Icn                            string          `json:"icn"`
	AccountEmail                   string          `json:"accountEmail"`
	DswOfferAccountingSystemCode   string          `json:"dswOfferAccountingSystemCode"`
	DswSubscriptionAgreementNumber string          `json:"dswSubscriptionAgreementNumber"`
	SsmSubscriptionId              string          `json:"ssmSubscriptionId"`
	MeasuredUsage                  []MeasuredUsage `json:"measuredUsage"`
}

// Mock function to generate data in Prometheus format
func mockPrometheusMetrics() string {
	report := MarketplaceReportData{
		EventID:                        "qradar-0",
		Start:                          time.Now().Add(-24*time.Hour).Unix() * 1000,
		End:                            time.Now().Unix() * 1000,
		Source:                         "ILMT",
		SourceSaas:                     "saas-xyz",
		LicensePartNumber:              "LPN-12345",
		ProductID:                      "1cf12bd6e33544609cf7766abebfc8ee",
		ProductName:                    "test product",
		Icn:                            "icn-67890",
		AccountEmail:                   "user@example.com",
		DswOfferAccountingSystemCode:   "offer-xyz",
		DswSubscriptionAgreementNumber: "sub-agreement-1234",
		SsmSubscriptionId:              "ssm-98765",
		MeasuredUsage: []MeasuredUsage{
			{
				MetricID:              "metricid",
				Value:                 4000,
				MetricType:            "license",
				MetricAggregationType: "sum",
			},
		},
	}

	// Format the mock data as Prometheus text format
	metrics := "# HELP marketplace_report_ru Marketplace report resource usage\n"
	metrics += "# TYPE marketplace_report_ru summary\n"

	for _, usage := range report.MeasuredUsage {
		metrics += fmt.Sprintf("marketplace_report_ru{event_id=\"%s\", product_name=\"%s\", account_email=\"%s\", metric_id=\"%s\", metric_type=\"%s\", aggregation_type=\"%s\", quantile=\"0.0\"} %f\n",
			report.EventID,
			report.ProductName,
			report.AccountEmail,
			usage.MetricID,
			usage.MetricType,
			usage.MetricAggregationType,
			usage.Value,
		)
	}

	return metrics
}

func prometheusMetricsHandler(w http.ResponseWriter, r *http.Request) {
	metrics := mockPrometheusMetrics()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(metrics))
}

func main() {
	http.HandleFunc("/metrics", prometheusMetricsHandler)

	port := ":9153"
	fmt.Printf("Server is running on http://localhost%s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
