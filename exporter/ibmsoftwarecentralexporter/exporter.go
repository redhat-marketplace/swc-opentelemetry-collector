// Copyright 2025 IBM Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ibmsoftwarecentralexporter // import "github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter"

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"

	"github.com/google/uuid"

	"github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter/v3alpha1"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

type baseExporter struct {
	config            *Config
	telemetrySettings component.TelemetrySettings
	logger            *zap.Logger
	client            *http.Client
	tarGzipPool       *TarGzipPool
}

// sendData encapsulates the common sending logic.
func (be *baseExporter) sendData(payloadBytes, manifestBytes []byte, incumbent interface{}) error {
	id := uuid.New()
	archive, err := be.tarGzipPool.TGZ(id.String(), manifestBytes, payloadBytes)
	if err != nil {
		return err
	}

	archiveReader := bytes.NewBuffer(archive)
	reqBody := &bytes.Buffer{}
	writer := multipart.NewWriter(reqBody)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="isce-%s"; filename="isce-%s.tar.gz"`, id, id))
	h.Set("Content-Type", "application/gzip")

	part, err := writer.CreatePart(h)
	if err != nil {
		return err
	}

	_, err = io.Copy(part, archiveReader)
	if err != nil {
		return err
	}

	if err = writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, be.config.Endpoint, reqBody)
	if err != nil {
		switch data := incumbent.(type) {
		case plog.Logs:
			return consumererror.NewLogs(err, data)
		case pmetric.Metrics:
			return err
		default:
			return err
		}
	}

	for k, v := range be.config.ClientConfig.Headers {
		req.Header.Set(k, string(v))
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	if be.client == nil {
		be.client = http.DefaultClient
	}

	res, err := be.client.Do(req)
	if err != nil {
		return err
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if err = res.Body.Close(); err != nil {
		return err
	}

	rerr := fmt.Errorf("remote write returned HTTP status %v; err = %w: %s", res.Status, err, string(resBody))
	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		return nil
	case res.StatusCode >= 500 && res.StatusCode < 600:
		return rerr
	case res.StatusCode == http.StatusTooManyRequests:
		return rerr
	default:
		return consumererror.NewPermanent(fmt.Errorf("line protocol write returned %q %q", res.Status, string(resBody)))
	}
}

type ibmsoftwarecentralexporter struct {
	baseExporter
}

type swcAccountMetricsExporter struct {
	baseExporter
}

func initExporter(cfg *Config, createSettings exporter.Settings) (*ibmsoftwarecentralexporter, error) {
	tarGzipPool := &TarGzipPool{}
	se := &ibmsoftwarecentralexporter{
		baseExporter: baseExporter{
			config:            cfg,
			telemetrySettings: createSettings.TelemetrySettings,
			logger:            createSettings.Logger,
			tarGzipPool:       tarGzipPool,
		},
	}
	se.logger.Info("IBM Software Central Exporter configured", zap.String("endpoint", cfg.Endpoint))
	return se, nil
}

func initSWCAccountMetricsExporter(cfg *Config, createSettings exporter.Settings) (*swcAccountMetricsExporter, error) {
	tarGzipPool := &TarGzipPool{}
	se := &swcAccountMetricsExporter{
		baseExporter: baseExporter{
			config:            cfg,
			telemetrySettings: createSettings.TelemetrySettings,
			logger:            createSettings.Logger,
			tarGzipPool:       tarGzipPool,
		},
	}
	se.logger.Info("SWC Account Metrics Exporter configured", zap.String("endpoint", cfg.Endpoint))
	return se, nil
}

func newLogsExporter(ctx context.Context, params exporter.Settings, cfg *Config) (exporter.Logs, error) {
	s, err := initExporter(cfg, params)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the logs exporter: %w", err)
	}
	return exporterhelper.NewLogs(
		ctx,
		params,
		cfg,
		s.pushLogsData,
		exporterhelper.WithTimeout(cfg.TimeoutSettings),
		exporterhelper.WithRetry(cfg.BackOffConfig),
		exporterhelper.WithQueue(cfg.QueueSettings),
		exporterhelper.WithStart(s.start),
		exporterhelper.WithShutdown(s.shutdown),
	)
}

func newMetricsExporter(ctx context.Context, params exporter.Settings, cfg *Config) (exporter.Metrics, error) {
	s, err := initSWCAccountMetricsExporter(cfg, params)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the metrics exporter: %w", err)
	}
	return exporterhelper.NewMetrics(ctx, params, cfg, s.pushMetrics)
}

func (se *ibmsoftwarecentralexporter) start(ctx context.Context, host component.Host) (err error) {
	if se.config.ClientConfig.Auth != nil {
		se.logger.Debug("auth", zap.String("clientConfig.Auth", fmt.Sprint(se.config.ClientConfig.Auth)))
	} else {
		se.logger.Debug("auth", zap.String("clientConfig.Auth", "isNil"))
	}
	se.client, err = se.config.ClientConfig.ToClient(ctx, host, se.telemetrySettings)
	return err
}

func (se *ibmsoftwarecentralexporter) shutdown(context.Context) error {
	if se.client != nil {
		se.client.CloseIdleConnections()
	}
	return nil
}

func (se *ibmsoftwarecentralexporter) pushLogsData(ctx context.Context, logs plog.Logs) error {
	eventJsons := []json.RawMessage{}
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		resourceLogs := logs.ResourceLogs().At(i)
		for j := 0; j < resourceLogs.ScopeLogs().Len(); j++ {
			scopeLogs := resourceLogs.ScopeLogs().At(j)
			for k := 0; k < scopeLogs.LogRecords().Len(); k++ {
				logRecord := scopeLogs.LogRecords().At(k)
				se.logger.Debug("logRecord", zap.String("body", logRecord.Body().AsString()))
				b := []byte(logRecord.Body().AsString())
				if json.Valid(b) {
					eventJsons = append(eventJsons, b)
				} else {
					se.logger.Debug("logRecord was not json", zap.String("body", logRecord.Body().AsString()))
				}
			}
		}
	}
	if len(eventJsons) > 0 {
		reportDataBytes, manifestBytes, err := se.buildLogsPayload(eventJsons)
		if err != nil {
			return err
		}
		return se.sendData(reportDataBytes, manifestBytes, logs)
	}
	return nil
}

func (se *ibmsoftwarecentralexporter) buildLogsPayload(eventJsons []json.RawMessage) ([]byte, []byte, error) {
	metadata := make(Metadata)
	reportData := ReportData{Metadata: metadata, EventJsons: eventJsons}
	reportDataBytes, err := json.Marshal(reportData)
	if err != nil {
		return nil, nil, err
	}
	se.logger.Debug("report data", zap.String("data", string(reportDataBytes)))
	manifest := Manifest{Type: "dataReporter", Version: "1"}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, err
	}
	return reportDataBytes, manifestBytes, nil
}

// pushMetrics iterates over metrics, transforms them, and sends the transformed metrics using sendData.
func (se *swcAccountMetricsExporter) pushMetrics(ctx context.Context, metrics pmetric.Metrics) error {
	for i := 0; i < metrics.ResourceMetrics().Len(); i++ {
		rm := metrics.ResourceMetrics().At(i)
		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			scopeMetrics := rm.ScopeMetrics().At(j)
			for k := 0; k < scopeMetrics.Metrics().Len(); k++ {
				metric := scopeMetrics.Metrics().At(k)
				se.logger.Info("Processing metric", zap.String("metric name", metric.Name()))
				if !validate(metric) {
					continue
				}
				swcMetrics := transformMetrics(metric)
				printTransformedMetrics(swcMetrics)
				data, manifestBytes, err := se.buildMetricsPayload(swcMetrics)
				if err != nil {
					se.logger.Error("failed to marshal transformed metrics", zap.Error(err))
					continue
				}
				if err := se.sendData(data, manifestBytes, metrics); err != nil {
					se.logger.Error("failed to send transformed metrics", zap.Error(err))
					continue
				}
			}
		}
	}
	return nil
}

func (se *swcAccountMetricsExporter) buildMetricsPayload(swcMetrics v3alpha1.MarketplaceReportSlice) ([]byte, []byte, error) {
	data, err := json.Marshal(swcMetrics)
	if err != nil {
		return nil, nil, err
	}
	manifest := Manifest{Type: "metricsReporter", Version: "1"}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, err
	}
	se.logger.Debug("metrics data", zap.String("data", string(data)))
	return data, manifestBytes, nil
}

func validate(metric pmetric.Metric) bool {
	if !validateReportDataFields(metric) {
		return false
	}
	if !validateMeasuredUsage(metric) {
		return false
	}
	return true
}

// validateReportDataFields checks that the required top-level fields (eventId, start, end, accountId)
// exist in the first datapoint's attribute map.
func validateReportDataFields(metric pmetric.Metric) bool {
	var attrs pcommon.Map
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		if metric.Gauge().DataPoints().Len() > 0 {
			attrs = metric.Gauge().DataPoints().At(0).Attributes()
		} else {
			log.Printf("Dropping metric: no gauge datapoints available")
			return false
		}
	case pmetric.MetricTypeSum:
		if metric.Sum().DataPoints().Len() > 0 {
			attrs = metric.Sum().DataPoints().At(0).Attributes()
		} else {
			log.Printf("Dropping metric: no sum datapoints available")
			return false
		}
	default:
		log.Printf("Dropping metric: unsupported metric type: %s", metric.Type().String())
		return false
	}

	var missing []string
	if getAttribute(attrs, "eventId", "") == "" {
		missing = append(missing, "eventId")
	}
	if getAttributeInt64(attrs, "start", 0) == 0 {
		missing = append(missing, "start")
	}
	if getAttributeInt64(attrs, "end", 0) == 0 {
		missing = append(missing, "end")
	}
	if getAttribute(attrs, "accountId", "") == "" {
		missing = append(missing, "accountId")
	}
	if len(missing) > 0 {
		log.Printf("Dropping metric due to missing usage event fields: %v", missing)
		return false
	}
	return true
}

func validateMeasuredUsage(metric pmetric.Metric) bool {
	validDPCount := 0
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		for i := 0; i < metric.Gauge().DataPoints().Len(); i++ {
			if validateMeausredUsageDataPoint(metric.Gauge().DataPoints().At(i)) {
				validDPCount++
			}
		}
	case pmetric.MetricTypeSum:
		for i := 0; i < metric.Sum().DataPoints().Len(); i++ {
			if validateMeausredUsageDataPoint(metric.Sum().DataPoints().At(i)) {
				validDPCount++
			}
		}
	default:
		log.Printf("Unsupported metric type: %s", metric.Type().String())
		return false
	}
	if validDPCount == 0 {
		log.Printf("Dropping metric: no measured usage datapoints passed validation (missing metricId and/or value)")
		return false
	}
	return true
}

func validateMeausredUsageDataPoint(dp pmetric.NumberDataPoint) bool {
	attrs := dp.Attributes()
	missing := []string{}
	if getDPAttribute(attrs, "metricId", "") == "" {
		missing = append(missing, "metricId")
	}
	_, hasValue := attrs.Get("value")
	if !hasValue {
		missing = append(missing, "value")
	}
	if len(missing) > 0 {
		log.Printf("Dropping measured usage datapoint: missing required fields: %v", missing)
		return false
	}
	return true
}

// transformMetrics builds the full transformed object.
func transformMetrics(metric pmetric.Metric) v3alpha1.MarketplaceReportSlice {
	reportData := transformMarketplaceReportData(metric)
	if reportData == nil {
		return v3alpha1.MarketplaceReportSlice{}
	}
	reportData.MeasuredUsage = transformMeasuredUsage(metric)
	return v3alpha1.MarketplaceReportSlice{
		Metadata: buildSourceMetadata(metric),
		Metrics:  []*v3alpha1.MarketplaceReportData{reportData},
	}
}

func buildSourceMetadata(metric pmetric.Metric) *v3alpha1.SourceMetadata {
	var attrs pcommon.Map
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		if metric.Gauge().DataPoints().Len() > 0 {
			attrs = metric.Gauge().DataPoints().At(0).Attributes()
		}
	case pmetric.MetricTypeSum:
		if metric.Sum().DataPoints().Len() > 0 {
			attrs = metric.Sum().DataPoints().At(0).Attributes()
		}
	default:
		attrs = pcommon.NewMap()
	}
	return &v3alpha1.SourceMetadata{
		ClusterID:     getAttribute(attrs, "clusterId", ""),
		AccountID:     getAttribute(attrs, "accountId", ""),
		Version:       getAttribute(attrs, "version", ""),
		ReportVersion: getAttribute(attrs, "reportVersion", ""),
		Environment:   getReportEnvironment(getAttribute(attrs, "environment", "")),
	}
}

// transformMarketplaceReportData extracts event-level fields from the first datapoint's attributes.
// We assume validation has already ensured required fields are present.
func transformMarketplaceReportData(metric pmetric.Metric) *v3alpha1.MarketplaceReportData {
	var attrs pcommon.Map
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		if metric.Gauge().DataPoints().Len() > 0 {
			attrs = metric.Gauge().DataPoints().At(0).Attributes()
		}
	case pmetric.MetricTypeSum:
		if metric.Sum().DataPoints().Len() > 0 {
			attrs = metric.Sum().DataPoints().At(0).Attributes()
		}
	default:
		attrs = pcommon.NewMap()
	}
	return &v3alpha1.MarketplaceReportData{
		EventID:                        getAttribute(attrs, "eventId", ""),
		IntervalStart:                  getAttributeInt64(attrs, "start", 0),
		IntervalEnd:                    getAttributeInt64(attrs, "end", 0),
		AccountID:                      getAttribute(attrs, "accountId", ""),
		SubscriptionId:                 getAttribute(attrs, "subscriptionId", ""),
		Source:                         getAttribute(attrs, "source", ""),
		SourceSaas:                     getAttribute(attrs, "sourceSaas", ""),
		AccountIdSaas:                  getAttribute(attrs, "accountIdSaas", ""),
		SubscriptionIdSaas:             getAttribute(attrs, "subscriptionIdSaas", ""),
		ProductType:                    getAttribute(attrs, "productType", ""),
		LicensePartNumber:              getAttribute(attrs, "licensePartNumber", ""),
		ProductId:                      getAttribute(attrs, "productId", ""),
		SapEntitlementLine:             getAttribute(attrs, "sapEntitlementLine", ""),
		ProductName:                    getAttribute(attrs, "productName", ""),
		ParentProductId:                getAttribute(attrs, "parentProductId", ""),
		ParentProductName:              getAttribute(attrs, "parentProductName", ""),
		ParentMetricId:                 getAttribute(attrs, "parentMetricId", ""),
		TopLevelProductId:              getAttribute(attrs, "topLevelProductId", ""),
		TopLevelProductName:            getAttribute(attrs, "topLevelProductName", ""),
		TopLevelProductMetricId:        getAttribute(attrs, "topLevelProductMetricId", ""),
		DswOfferAccountingSystemCode:   getAttribute(attrs, "dswOfferAccountingSystemCode", ""),
		DswSubscriptionAgreementNumber: getAttribute(attrs, "dswSubscriptionAgreementNumber", ""),
		SsmSubscriptionId:              getAttribute(attrs, "ssmSubscriptionId", ""),
		ICN:                            getAttribute(attrs, "ICN", ""),
		Group:                          getAttribute(attrs, "group", ""),
		GroupName:                      getAttribute(attrs, "groupName", ""),
		Kind:                           getAttribute(attrs, "kind", ""),
	}
}

// transformMeasuredUsage extracts measured usage datapoints from the metric.
func transformMeasuredUsage(metric pmetric.Metric) []v3alpha1.MeasuredUsage {
	var usageList []v3alpha1.MeasuredUsage

	convertDataPoint := func(dp pmetric.NumberDataPoint) (v3alpha1.MeasuredUsage, bool) {
		attrs := dp.Attributes()
		return v3alpha1.MeasuredUsage{
			MetricID:               getDPAttribute(attrs, "metricId", ""),
			Value:                  getFloat64FromAttribute(attrs, "value", dp.DoubleValue()),
			MeterDefNamespace:      getDPAttribute(attrs, "meter_def_namespace", ""),
			MeterDefName:           getDPAttribute(attrs, "meter_def_name", ""),
			MetricType:             getDPAttribute(attrs, "metricType", ""),
			MetricAggregationType:  getDPAttribute(attrs, "metricAggregationType", ""),
			MeasuredMetricId:       getDPAttribute(attrs, "measuredMetricId", ""),
			ProductConversionRatio: getDPAttribute(attrs, "productConversionRatio", ""),
			MeasuredValue:          getDPAttribute(attrs, "measuredValue", ""),
			ClusterId:              getDPAttribute(attrs, "clusterId", ""),
			Hostname:               getDPAttribute(attrs, "hostname", ""),
			Pod:                    getDPAttribute(attrs, "pod", ""),
			PlatformId:             getDPAttribute(attrs, "platformId", ""),
			Crn:                    getDPAttribute(attrs, "crn", ""),
			IsViewable:             getDPAttribute(attrs, "isViewable", ""),
			CalculateSummary:       getDPAttribute(attrs, "calculateSummary", ""),
		}, true
	}

	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		g := metric.Gauge()
		for i := 0; i < g.DataPoints().Len(); i++ {
			dp := g.DataPoints().At(i)
			if usage, ok := convertDataPoint(dp); ok {
				usageList = append(usageList, usage)
			} else {
				log.Printf("Dropping measured usage datapoint due to missing required fields (metricId and/or value)")
			}
		}
	case pmetric.MetricTypeSum:
		s := metric.Sum()
		for i := 0; i < s.DataPoints().Len(); i++ {
			dp := s.DataPoints().At(i)
			if usage, ok := convertDataPoint(dp); ok {
				usageList = append(usageList, usage)
			} else {
				log.Printf("Dropping measured usage datapoint due to missing required fields (metricId and/or value)")
			}
		}
	default:
		log.Printf("unsupported metric type: %s", metric.Type().String())
	}

	return usageList
}

func getDPAttribute(attrs pcommon.Map, key, defaultValue string) string {
	if val, ok := attrs.Get(key); ok {
		return val.AsString()
	}
	return defaultValue
}

func getFloat64FromAttribute(attrs pcommon.Map, key string, defaultValue float64) float64 {
	if v, ok := attrs.Get(key); ok {
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			return v.Double()
		case pcommon.ValueTypeInt:
			return float64(v.Int())
		case pcommon.ValueTypeStr:
			if f, err := strconv.ParseFloat(v.AsString(), 64); err == nil {
				return f
			}
		}
	}
	return defaultValue
}

func getAttribute(attrs pcommon.Map, key string, defaultValue string) string {
	if val, ok := attrs.Get(key); ok {
		return val.AsString()
	}
	return defaultValue
}

func getAttributeInt64(attrs pcommon.Map, key string, defaultValue int64) int64 {
	if val, ok := attrs.Get(key); ok {
		switch val.Type() {
		case pcommon.ValueTypeInt:
			return val.Int()
		case pcommon.ValueTypeDouble:
			return int64(val.Double())
		case pcommon.ValueTypeStr:
			if i, err := strconv.ParseInt(val.AsString(), 10, 64); err == nil {
				return i
			}
		}
	}
	return defaultValue
}

func getReportEnvironment(environmentStr string) v3alpha1.ReportEnvironment {
	switch environmentStr {
	case string(v3alpha1.ReportProductionEnv):
		return v3alpha1.ReportProductionEnv
	case string(v3alpha1.ReportSandboxEnv):
		return v3alpha1.ReportSandboxEnv
	default:
		return ""
	}
}

func printTransformedMetrics(swcMetrics v3alpha1.MarketplaceReportSlice) {
	b, err := json.MarshalIndent(swcMetrics, "", "  ")
	if err != nil {
		log.Printf("error marshalling metrics: %v", err)
		return
	}
	fmt.Printf("Transformed SWC Account Metrics:\n%s\n", string(b))
}
