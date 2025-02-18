package main

import (
	"fmt"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

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

// structToMap converts a struct to a map[string]string using the `json` tag.
func structToMap(v interface{}) map[string]string {
	m := make(map[string]string)
	val := reflect.ValueOf(v)
	typ := reflect.TypeOf(v)
	if typ.Kind() == reflect.Ptr {
		val = val.Elem()
		typ = typ.Elem()
	}
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		key := tag
		if comma := strings.Index(tag, ","); comma != -1 {
			key = tag[:comma]
		}
		fieldVal := val.Field(i)
		var str string
		switch fieldVal.Kind() {
		case reflect.String:
			str = fieldVal.String()
		case reflect.Int, reflect.Int64:
			str = strconv.FormatInt(fieldVal.Int(), 10)
		case reflect.Float64:
			str = strconv.FormatFloat(fieldVal.Float(), 'f', -1, 64)
		default:
			str = fmt.Sprintf("%v", fieldVal.Interface())
		}
		m[key] = str
	}
	return m
}

func mergeMaps(a, b map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range a {
		merged[k] = v
	}
	for k, v := range b {
		merged[k] = v
	}
	return merged
}

type mockCollector struct {
	report     MarketplaceReportData
	metadata   SourceMetadata
	usages     []MeasuredUsage
	metricType string

	eventLabels        map[string]string
	combinedLabelNames []string
	collector          prometheus.Collector
}

// buildEventLabels converts the report and metadata into event-level labels.
func (mc *mockCollector) buildEventLabels() {
	mc.eventLabels = mergeMaps(structToMap(mc.report), structToMap(mc.metadata))
}

// buildWithUsageLabels builds the union of label names from event-level data and usage data.
func (mc *mockCollector) buildWithUsageLabels() {
	union := make(map[string]struct{})
	for k := range mc.eventLabels {
		union[k] = struct{}{}
	}
	if len(mc.usages) > 0 {
		usageMap := structToMap(mc.usages[0])
		for k := range usageMap {
			union[k] = struct{}{}
		}
	}
	var keys []string
	for k := range union {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	mc.combinedLabelNames = keys
}

// createCollector creates the Prometheus collector based on the metricType and label names.
func (mc *mockCollector) createCollector() {
	switch mc.metricType {
	case "gauge":
		mc.collector = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "marketplace_report_ru",
			Help: "Marketplace report resource usage",
		}, mc.combinedLabelNames)
	case "summary":
		mc.collector = prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "marketplace_report_ru",
			Help: "Marketplace report resource usage",
		}, mc.combinedLabelNames)
	case "counter":
		mc.collector = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "marketplace_report_ru",
			Help: "Marketplace report resource usage",
		}, mc.combinedLabelNames)
	default:
		log.Printf("Unsupported metric type '%s', defaulting to gauge", mc.metricType)
		mc.collector = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "marketplace_report_ru",
			Help: "Marketplace report resource usage",
		}, mc.combinedLabelNames)
	}
}

// setMetricValues sets the metric values on the collector by merging event-level labels with usage labels.
func (mc *mockCollector) setMetricValues() {
	switch mc.metricType {
	case "gauge":
		vec := mc.collector.(*prometheus.GaugeVec)
		for _, usage := range mc.usages {
			usageLabels := structToMap(usage)
			merged := mergeMaps(mc.eventLabels, usageLabels)
			vec.With(merged).Set(usage.Value)
		}
	case "summary":
		vec := mc.collector.(*prometheus.SummaryVec)
		for _, usage := range mc.usages {
			usageLabels := structToMap(usage)
			merged := mergeMaps(mc.eventLabels, usageLabels)
			vec.With(merged).Observe(usage.Value)
		}
	case "counter":
		vec := mc.collector.(*prometheus.CounterVec)
		for _, usage := range mc.usages {
			usageLabels := structToMap(usage)
			merged := mergeMaps(mc.eventLabels, usageLabels)
			vec.With(merged).Add(usage.Value)
		}
	default:
		vec := mc.collector.(*prometheus.GaugeVec)
		for _, usage := range mc.usages {
			usageLabels := structToMap(usage)
			merged := mergeMaps(mc.eventLabels, usageLabels)
			vec.With(merged).Set(usage.Value)
		}
	}
}

func (mc *mockCollector) BuildCollector() prometheus.Collector {
	mc.buildEventLabels()
	mc.buildWithUsageLabels()
	mc.createCollector()
	mc.setMetricValues()
	return mc.collector
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
			MetricType:             "license", // can be overridden by transformation if needed
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

	// Choose the metric type: "gauge", "summary", or "counter".
	metricType := "gauge" // change as needed

	mc := mockCollector{
		report:     report,
		metadata:   metadata,
		usages:     usages,
		metricType: metricType,
	}

	collector := mc.BuildCollector()

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	port := ":9153"
	fmt.Printf("Server is running on http://localhost%s/metrics\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
