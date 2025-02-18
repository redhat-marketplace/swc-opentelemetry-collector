// package main

// import (
// 	"fmt"
// 	"log"
// 	"net/http"

// 	"github.com/prometheus/client_golang/prometheus"
// 	"github.com/prometheus/client_golang/prometheus/promhttp"
// )

// /* using a gauge vector for now but can change to summary or some other type*/
// var marketplaceReport = prometheus.NewGaugeVec(
// 	prometheus.GaugeOpts{
// 		Name: "marketplace_report_ru",
// 		Help: "Marketplace report resource usage",
// 	},
// 	[]string{
// 		// MarketplaceReportData fields:
// 		"eventId",   // required
// 		"start",     // required (interval start in ms)
// 		"end",       // required (interval end in ms)
// 		"accountId", // required
// 		"subscriptionId",
// 		"source",
// 		"sourceSaas",
// 		"accountIdSaas",
// 		"subscriptionIdSaas",
// 		"productType",
// 		"licensePartNumber",
// 		"productId",
// 		"sapEntitlementLine",
// 		"productName",
// 		"parentProductId",
// 		"parentProductName",
// 		"parentMetricId",
// 		"topLevelProductId",
// 		"topLevelProductName",
// 		"topLevelProductMetricId",
// 		"dswOfferAccountingSystemCode",
// 		"dswSubscriptionAgreementNumber",
// 		"ssmSubscriptionId",
// 		"ICN",
// 		"group",
// 		"groupName",
// 		"kind",
// 		// SourceMetadata fields:
// 		"clusterId",
// 		"environment",
// 		"version",
// 		"reportVersion",
// 		// MeasuredUsage fields:
// 		"metricId", // required
// 		"value",    // required
// 		"meter_def_namespace",
// 		"meter_def_name",
// 		"metricType",
// 		"metricAggregationType",
// 		"measuredMetricId",
// 		"productConversionRatio",
// 		"measuredValue",
// 		"hostname",
// 		"pod",
// 		"platformId",
// 		"crn",
// 		"isViewable",
// 		"calculateSummary",
// 	},
// )

// func simulateMetrics() {
// 	// --- MarketplaceReportData & SourceMetadata ---
// 	labelsEvent := map[string]string{
// 		"eventId":                        "eventId",       // required
// 		"start":                          "1630000000000", // required
// 		"end":                            "1630003600000", // required
// 		"accountId":                      "account1",      // required
// 		"subscriptionId":                 "sub1",
// 		"source":                         "source1",
// 		"sourceSaas":                     "sourceSaas1",
// 		"accountIdSaas":                  "accountIdSaas1",
// 		"subscriptionIdSaas":             "subscriptionIdSaas1",
// 		"productType":                    "type1",
// 		"licensePartNumber":              "LPN1",
// 		"productId":                      "prod1",
// 		"sapEntitlementLine":             "sapLine1",
// 		"productName":                    "test product",
// 		"parentProductId":                "parentProd1",
// 		"parentProductName":              "parent product name",
// 		"parentMetricId":                 "parentMetric1",
// 		"topLevelProductId":              "topLevelProd1",
// 		"topLevelProductName":            "top level product name",
// 		"topLevelProductMetricId":        "topLevelMetric1",
// 		"dswOfferAccountingSystemCode":   "offerCode1",
// 		"dswSubscriptionAgreementNumber": "subscriptionAgreement1",
// 		"ssmSubscriptionId":              "ssmSub1",
// 		"ICN":                            "ICN1",
// 		"group":                          "group1",
// 		"groupName":                      "group name 1",
// 		"kind":                           "kind1",
// 		// SourceMetadata fields:
// 		"clusterId":     "cluster1",
// 		"environment":   "production",
// 		"version":       "v1.0.0",
// 		"reportVersion": "1",
// 	}

// 	// --- MeasuredUsage datapoint 1 fields ---
// 	labelsUsage1 := map[string]string{
// 		"metricId":               "usageMetric1", // required
// 		"value":                  "100",          // required, as string
// 		"meter_def_namespace":    "ns1",
// 		"meter_def_name":         "meter1",
// 		"metricType":             "license",
// 		"metricAggregationType":  "sum",
// 		"measuredMetricId":       "measuredMetric1",
// 		"productConversionRatio": "1.0",
// 		"measuredValue":          "value1",
// 		"hostname":               "host1",
// 		"pod":                    "pod1",
// 		"platformId":             "platform1",
// 		"crn":                    "crn1",
// 		"isViewable":             "true",
// 		"calculateSummary":       "true",
// 	}

// 	// --- MeasuredUsage datapoint 2 fields ---
// 	labelsUsage2 := map[string]string{
// 		"metricId":               "usageMetric2", // required
// 		"value":                  "200",          // required, as string
// 		"meter_def_namespace":    "ns2",
// 		"meter_def_name":         "meter2",
// 		"metricType":             "usage",
// 		"metricAggregationType":  "average",
// 		"measuredMetricId":       "measuredMetric2",
// 		"productConversionRatio": "1.0",
// 		"measuredValue":          "value2",
// 		"hostname":               "host1",
// 		"pod":                    "pod1",
// 		"platformId":             "platform1",
// 		"crn":                    "crn1",
// 		"isViewable":             "true",
// 		"calculateSummary":       "true",
// 	}

// 	marketplaceReport.Reset()

// 	// Combine event-level labels with measured usage labels for datapoint 1.
// 	labels1 := make(prometheus.Labels)
// 	for k, v := range labelsEvent {
// 		labels1[k] = v
// 	}
// 	for k, v := range labelsUsage1 {
// 		labels1[k] = v
// 	}

// 	// Combine event-level labels with measured usage labels for datapoint 2.
// 	labels2 := make(prometheus.Labels)
// 	for k, v := range labelsEvent {
// 		labels2[k] = v
// 	}
// 	for k, v := range labelsUsage2 {
// 		labels2[k] = v
// 	}

// 	// Set the gauge values for each datapoint.
// 	marketplaceReport.With(labels1).Set(100)
// 	marketplaceReport.With(labels2).Set(200)
// }

// func main() {

// 	simulateMetrics()

// 	// Create a custom registry and register only our metric.
// 	registry := prometheus.NewRegistry()
// 	registry.MustRegister(marketplaceReport)

// 	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

//		port := ":9153"
//		fmt.Printf("Server is running on http://localhost%s/metrics\n", port)
//		if err := http.ListenAndServe(port, nil); err != nil {
//			log.Fatalf("Error starting server: %v", err)
//		}
//	}
package main
