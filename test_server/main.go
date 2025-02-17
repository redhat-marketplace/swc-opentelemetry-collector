package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ReportEnvironment string

const (
	ReportProductionEnv ReportEnvironment = "production"
	ReportSandboxEnv    ReportEnvironment = "stage"
)

type SourceMetadata struct {
	ClusterID     string            `json:"clusterId" mapstructure:"clusterId"`
	AccountID     string            `json:"accountId,omitempty" mapstructure:"accountId"`
	Environment   ReportEnvironment `json:"environment,omitempty" mapstructure:"environment,omitempty"`
	Version       string            `json:"version,omitempty" mapstructure:"version,omitempty"`
	ReportVersion string            `json:"reportVersion,omitempty"`
}

type MarketplaceReportData struct {
	EventID                        string `json:"eventId" mapstructure:"-"`
	IntervalStart                  int64  `json:"start,omitempty" mapstructure:"-"`
	IntervalEnd                    int64  `json:"end,omitempty" mapstructure:"-"`
	AccountID                      string `json:"accountId,omitempty" mapstructure:"-"`
	SubscriptionId                 string `json:"subscriptionId,omitempty" mapstructure:"subscriptionId"`
	Source                         string `json:"source,omitempty" mapstructure:"source"`
	SourceSaas                     string `json:"sourceSaas,omitempty" mapstructure:"sourceSaas"`
	AccountIdSaas                  string `json:"accountIdSaas,omitempty" mapstructure:"accountIdSaas"`
	SubscriptionIdSaas             string `json:"subscriptionIdSaas,omitempty" mapstructure:"subscriptionIdSaas"`
	ProductType                    string `json:"productType,omitempty" mapstructure:"productType"`
	LicensePartNumber              string `json:"licensePartNumber,omitempty" mapstructure:"licensePartNumber"`
	ProductId                      string `json:"productId,omitempty" mapstructure:"productId"`
	SapEntitlementLine             string `json:"sapEntitlementLine,omitempty" mapstructure:"sapEntitlementLine"`
	ProductName                    string `json:"productName,omitempty" mapstructure:"productName"`
	ParentProductId                string `json:"parentProductId,omitempty" mapstructure:"parentProductId"`
	ParentProductName              string `json:"parentProductName,omitempty" mapstructure:"parentProductName"`
	ParentMetricId                 string `json:"parentMetricId,omitempty" mapstructure:"parentMetricId"`
	TopLevelProductId              string `json:"topLevelProductId,omitempty" mapstructure:"topLevelProductId"`
	TopLevelProductName            string `json:"topLevelProductName,omitempty" mapstructure:"topLevelProductName"`
	TopLevelProductMetricId        string `json:"topLevelProductMetricId,omitempty" mapstructure:"topLevelProductMetricId"`
	DswOfferAccountingSystemCode   string `json:"dswOfferAccountingSystemCode,omitempty" mapstructure:"dswOfferAccountingSystemCode"`
	DswSubscriptionAgreementNumber string `json:"dswSubscriptionAgreementNumber,omitempty" mapstructure:"dswSubscriptionAgreementNumber"`
	SsmSubscriptionId              string `json:"ssmSubscriptionId,omitempty" mapstructure:"ssmSubscriptionId"`
	ICN                            string `json:"ICN,omitempty" mapstructure:"ICN"`
	Group                          string `json:"group,omitempty" mapstructure:"group"`
	GroupName                      string `json:"groupName,omitempty" mapstructure:"groupName"`
	Kind                           string `json:"kind,omitempty" mapstructure:"kind"`
}

type MeasuredUsage struct {
	MetricID               string  `json:"metricId" mapstructure:"-"`
	Value                  float64 `json:"value" mapstructure:"-"`
	MeterDefNamespace      string  `json:"meter_def_namespace,omitempty" mapstructure:"meter_def_namespace"`
	MeterDefName           string  `json:"meter_def_name,omitempty" mapstructure:"meter_def_name"`
	MetricType             string  `json:"metricType,omitempty" mapstructure:"metricType"`
	MetricAggregationType  string  `json:"metricAggregationType,omitempty" mapstructure:"metricAggregationType"`
	MeasuredMetricId       string  `json:"measuredMetricId,omitempty" mapstructure:"measuredMetricId"`
	ProductConversionRatio string  `json:"productConversionRatio,omitempty" mapstructure:"productConversionRatio"`
	MeasuredValue          string  `json:"measuredValue,omitempty" mapstructure:"measuredValue"`
	Hostname               string  `json:"hostname,omitempty" mapstructure:"hostname"`
	Pod                    string  `json:"pod,omitempty" mapstructure:"pod"`
	PlatformId             string  `json:"platformId,omitempty" mapstructure:"platformId"`
	Crn                    string  `json:"crn,omitempty" mapstructure:"crn"`
	IsViewable             string  `json:"isViewable,omitempty" mapstructure:"isViewable"`
	CalculateSummary       string  `json:"calculateSummary,omitempty" mapstructure:"calculateSummary"`
}

func buildEventLabels(report MarketplaceReportData, metadata SourceMetadata) map[string]string {
	return map[string]string{
		"eventId":                        report.EventID,
		"start":                          strconv.FormatInt(report.IntervalStart, 10),
		"end":                            strconv.FormatInt(report.IntervalEnd, 10),
		"accountId":                      report.AccountID,
		"subscriptionId":                 report.SubscriptionId,
		"source":                         report.Source,
		"sourceSaas":                     report.SourceSaas,
		"accountIdSaas":                  report.AccountIdSaas,
		"subscriptionIdSaas":             report.SubscriptionIdSaas,
		"productType":                    report.ProductType,
		"licensePartNumber":              report.LicensePartNumber,
		"productId":                      report.ProductId,
		"sapEntitlementLine":             report.SapEntitlementLine,
		"productName":                    report.ProductName,
		"parentProductId":                report.ParentProductId,
		"parentProductName":              report.ParentProductName,
		"parentMetricId":                 report.ParentMetricId,
		"topLevelProductId":              report.TopLevelProductId,
		"topLevelProductName":            report.TopLevelProductName,
		"topLevelProductMetricId":        report.TopLevelProductMetricId,
		"dswOfferAccountingSystemCode":   report.DswOfferAccountingSystemCode,
		"dswSubscriptionAgreementNumber": report.DswSubscriptionAgreementNumber,
		"ssmSubscriptionId":              report.SsmSubscriptionId,
		"ICN":                            report.ICN,
		"group":                          report.Group,
		"groupName":                      report.GroupName,
		"kind":                           report.Kind,
		// SourceMetadata fields:
		"clusterId":     metadata.ClusterID,
		"environment":   string(metadata.Environment),
		"version":       metadata.Version,
		"reportVersion": metadata.ReportVersion,
	}
}

func buildUsageLabels(usage MeasuredUsage) map[string]string {
	return map[string]string{
		"metricId":               usage.MetricID,
		"value":                  fmt.Sprintf("%.0f", usage.Value),
		"meter_def_namespace":    usage.MeterDefNamespace,
		"meter_def_name":         usage.MeterDefName,
		"metricType":             usage.MetricType,
		"metricAggregationType":  usage.MetricAggregationType,
		"measuredMetricId":       usage.MeasuredMetricId,
		"productConversionRatio": usage.ProductConversionRatio,
		"measuredValue":          usage.MeasuredValue,
		"hostname":               usage.Hostname,
		"pod":                    usage.Pod,
		"platformId":             usage.PlatformId,
		"crn":                    usage.Crn,
		"isViewable":             usage.IsViewable,
		"calculateSummary":       usage.CalculateSummary,
	}
}

// mergeLabels merges two maps into a new map.
func mergeLabels(a, b map[string]string) prometheus.Labels {
	merged := make(prometheus.Labels)
	for k, v := range a {
		merged[k] = v
	}
	for k, v := range b {
		merged[k] = v
	}
	return merged
}

func simulateMetrics(report MarketplaceReportData, metadata SourceMetadata, usages []MeasuredUsage, metricType string) prometheus.Collector {
	eventLabels := buildEventLabels(report, metadata)

	// Define the label names (order must match the Collector creation).
	labelNames := []string{
		// MarketplaceReportData fields:
		"eventId", "start", "end", "accountId", "subscriptionId", "source", "sourceSaas", "accountIdSaas", "subscriptionIdSaas",
		"productType", "licensePartNumber", "productId", "sapEntitlementLine", "productName", "parentProductId", "parentProductName",
		"parentMetricId", "topLevelProductId", "topLevelProductName", "topLevelProductMetricId", "dswOfferAccountingSystemCode",
		"dswSubscriptionAgreementNumber", "ssmSubscriptionId", "ICN", "group", "groupName", "kind",
		// SourceMetadata fields:
		"clusterId", "environment", "version", "reportVersion",
		// MeasuredUsage fields:
		"metricId", "value", "meter_def_namespace", "meter_def_name", "metricType", "metricAggregationType",
		"measuredMetricId", "productConversionRatio", "measuredValue", "hostname", "pod", "platformId", "crn", "isViewable", "calculateSummary",
	}

	var collector prometheus.Collector
	switch metricType {
	case "gauge":
		collector = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "marketplace_report_ru",
			Help: "Marketplace report resource usage",
		}, labelNames)
	case "summary":
		collector = prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "marketplace_report_ru",
			Help: "Marketplace report resource usage",
		}, labelNames)
	case "counter":
		collector = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "marketplace_report_ru",
			Help: "Marketplace report resource usage",
		}, labelNames)
	default:
		log.Printf("Unsupported metric type '%s', defaulting to gauge", metricType)
		collector = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "marketplace_report_ru",
			Help: "Marketplace report resource usage",
		}, labelNames)
	}

	switch metricType {
	case "gauge":
		vec := collector.(*prometheus.GaugeVec)
		for _, usage := range usages {
			usageLabels := buildUsageLabels(usage)
			merged := mergeLabels(eventLabels, usageLabels)
			vec.With(merged).Set(usage.Value)
		}
	case "summary":
		vec := collector.(*prometheus.SummaryVec)
		for _, usage := range usages {
			usageLabels := buildUsageLabels(usage)
			merged := mergeLabels(eventLabels, usageLabels)
			vec.With(merged).Observe(usage.Value)
		}
	case "counter":
		vec := collector.(*prometheus.CounterVec)
		for _, usage := range usages {
			usageLabels := buildUsageLabels(usage)
			merged := mergeLabels(eventLabels, usageLabels)
			vec.With(merged).Add(usage.Value)
		}
	default:
		vec := collector.(*prometheus.GaugeVec)
		for _, usage := range usages {
			usageLabels := buildUsageLabels(usage)
			merged := mergeLabels(eventLabels, usageLabels)
			vec.With(merged).Set(usage.Value)
		}
	}

	return collector
}

func main() {
	report := MarketplaceReportData{
		EventID:                        "eventId",
		IntervalStart:                  1630000000000,
		IntervalEnd:                    1630003600000,
		AccountID:                      "account1",
		SubscriptionId:                 "sub1",
		Source:                         "source1",
		SourceSaas:                     "sourceSaas1",
		AccountIdSaas:                  "accountIdSaas1",
		SubscriptionIdSaas:             "subscriptionIdSaas1",
		ProductType:                    "type1",
		LicensePartNumber:              "LPN1",
		ProductId:                      "prod1",
		SapEntitlementLine:             "sapLine1",
		ProductName:                    "test product",
		ParentProductId:                "parentProd1",
		ParentProductName:              "parent product name",
		ParentMetricId:                 "parentMetric1",
		TopLevelProductId:              "topLevelProd1",
		TopLevelProductName:            "top level product name",
		TopLevelProductMetricId:        "topLevelMetric1",
		DswOfferAccountingSystemCode:   "offerCode1",
		DswSubscriptionAgreementNumber: "subscriptionAgreement1",
		SsmSubscriptionId:              "ssmSub1",
		ICN:                            "ICN1",
		Group:                          "group1",
		GroupName:                      "group name 1",
		Kind:                           "kind1",
	}

	metadata := SourceMetadata{
		ClusterID:     "cluster1",
		AccountID:     "account1",
		Environment:   ReportProductionEnv,
		Version:       "v1.0.0",
		ReportVersion: "1",
	}

	usages := []MeasuredUsage{
		{
			MetricID:               "usageMetric1",
			Value:                  100,
			MeterDefNamespace:      "ns1",
			MeterDefName:           "meter1",
			MetricType:             "license", // Although we may override this in transformation
			MetricAggregationType:  "sum",
			MeasuredMetricId:       "measuredMetric1",
			ProductConversionRatio: "1.0",
			MeasuredValue:          "value1",
			Hostname:               "host1",
			Pod:                    "pod1",
			PlatformId:             "platform1",
			Crn:                    "crn1",
			IsViewable:             "true",
			CalculateSummary:       "true",
		},
		{
			MetricID:               "usageMetric2",
			Value:                  200,
			MeterDefNamespace:      "ns2",
			MeterDefName:           "meter2",
			MetricType:             "usage",
			MetricAggregationType:  "average",
			MeasuredMetricId:       "measuredMetric2",
			ProductConversionRatio: "1.0",
			MeasuredValue:          "value2",
			Hostname:               "host1",
			Pod:                    "pod1",
			PlatformId:             "platform1",
			Crn:                    "crn1",
			IsViewable:             "true",
			CalculateSummary:       "true",
		},
	}

	// Choose the metric type: "gauge", "summary", or "counter"
	metricType := "gauge"

	collector := simulateMetrics(report, metadata, usages, metricType)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	port := ":9153"
	fmt.Printf("Server is running on http://localhost%s/metrics\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
