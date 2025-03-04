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

package ibmsoftwarecentralexporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"

	"github.com/google/uuid"

	dataReporterV1 "github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter/api/dataReporter/v1"
	swcAccountMetricsV1 "github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter/api/swcAccountMetrics/v1"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// ibmsoftwarecentralexporter is the unified exporter type for both logs and metrics.
type ibmsoftwarecentralexporter struct {
	config            *Config
	telemetrySettings component.TelemetrySettings
	logger            *zap.Logger
	client            *http.Client
	tarGzipPool       *TarGzipPool
}

// sendData encapsulates the common sending logic.
func (exp *ibmsoftwarecentralexporter) sendData(payloadBytes, manifestBytes []byte, incumbent interface{}) error {
	id := uuid.New()
	archive, err := exp.tarGzipPool.TGZ(id.String(), manifestBytes, payloadBytes)
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

	req, err := http.NewRequest(http.MethodPost, exp.config.Endpoint, reqBody)
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

	for k, v := range exp.config.ClientConfig.Headers {
		req.Header.Set(k, string(v))
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if exp.client == nil {
		exp.client = http.DefaultClient
	}

	exp.logger.Debug("Sending request", zap.String("endpoint", exp.config.Endpoint))
	res, err := exp.client.Do(req)
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

// newExporter creates a new ibmsoftwarecentralexporter.
func newExporter(cfg *Config, settings exporter.Settings) (*ibmsoftwarecentralexporter, error) {
	tarGzipPool := &TarGzipPool{}
	exp := &ibmsoftwarecentralexporter{
		config:            cfg,
		telemetrySettings: settings.TelemetrySettings,
		logger:            settings.Logger,
		tarGzipPool:       tarGzipPool,
	}
	exp.logger.Info("IBM Software Central Exporter configured", zap.String("endpoint", cfg.Endpoint))
	return exp, nil
}

func newLogsExporter(ctx context.Context, params exporter.Settings, cfg *Config) (exporter.Logs, error) {
	exp, err := newExporter(cfg, params)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the logs exporter: %w", err)
	}
	return exporterhelper.NewLogs(
		ctx,
		params,
		cfg,
		exp.pushLogsData,
		exporterhelper.WithTimeout(cfg.TimeoutSettings),
		exporterhelper.WithRetry(cfg.BackOffConfig),
		exporterhelper.WithQueue(cfg.QueueSettings),
		exporterhelper.WithStart(exp.start),
		exporterhelper.WithShutdown(exp.shutdown),
	)
}

func newMetricsExporter(ctx context.Context, params exporter.Settings, cfg *Config) (exporter.Metrics, error) {
	exp, err := newExporter(cfg, params)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the metrics exporter: %w", err)
	}
	return exporterhelper.NewMetrics(ctx, params, cfg, exp.pushMetrics)
}

func (exp *ibmsoftwarecentralexporter) start(ctx context.Context, host component.Host) error {
	if exp.config.ClientConfig.Auth != nil {
		exp.logger.Debug("auth", zap.String("clientConfig.Auth", fmt.Sprint(exp.config.ClientConfig.Auth)))
	} else {
		exp.logger.Debug("auth", zap.String("clientConfig.Auth", "isNil"))
	}
	var err error
	exp.client, err = exp.config.ClientConfig.ToClient(ctx, host, exp.telemetrySettings)
	return err
}

func (exp *ibmsoftwarecentralexporter) shutdown(ctx context.Context) error {
	if exp.client != nil {
		exp.client.CloseIdleConnections()
	}
	return nil
}

func (exp *ibmsoftwarecentralexporter) pushLogsData(ctx context.Context, logs plog.Logs) error {
	eventJsons := []json.RawMessage{}
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		resourceLogs := logs.ResourceLogs().At(i)
		for j := 0; j < resourceLogs.ScopeLogs().Len(); j++ {
			scopeLogs := resourceLogs.ScopeLogs().At(j)
			for k := 0; k < scopeLogs.LogRecords().Len(); k++ {
				logRecord := scopeLogs.LogRecords().At(k)
				exp.logger.Debug("logRecord", zap.String("body", logRecord.Body().AsString()))
				b := []byte(logRecord.Body().AsString())
				if json.Valid(b) {
					eventJsons = append(eventJsons, b)
				} else {
					exp.logger.Debug("logRecord was not json", zap.String("body", logRecord.Body().AsString()))
				}
			}
		}
	}
	if len(eventJsons) > 0 {
		reportDataBytes, manifestBytes, err := exp.buildLogsPayload(eventJsons)
		if err != nil {
			return err
		}
		return exp.sendData(reportDataBytes, manifestBytes, logs)
	}
	return nil
}

func (exp *ibmsoftwarecentralexporter) buildLogsPayload(eventJsons []json.RawMessage) ([]byte, []byte, error) {
	metadata := make(dataReporterV1.Metadata)
	reportData := dataReporterV1.ReportData{Metadata: metadata, EventJsons: eventJsons}
	reportDataBytes, err := json.Marshal(reportData)
	if err != nil {
		return nil, nil, err
	}
	exp.logger.Debug("report data", zap.String("data", string(reportDataBytes)))
	manifest := dataReporterV1.Manifest{Type: "dataReporter", Version: "1"}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, err
	}
	return reportDataBytes, manifestBytes, nil
}

func (exp *ibmsoftwarecentralexporter) pushMetrics(ctx context.Context, metrics pmetric.Metrics) error {
	for i := 0; i < metrics.ResourceMetrics().Len(); i++ {
		rm := metrics.ResourceMetrics().At(i)
		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			scopeMetrics := rm.ScopeMetrics().At(j)
			for k := 0; k < scopeMetrics.Metrics().Len(); k++ {
				metric := scopeMetrics.Metrics().At(k)
				exp.logger.Info("Processing metric", zap.String("metric name", metric.Name()))

				if errs := validate(metric); len(errs) > 0 {
					exp.logger.Debug("Validation errors", zap.Strings("errors", errs))
					continue
				}

				swcMetrics := transformMetrics(metric)
				logTransformedMetrics(exp.logger, swcMetrics)
				data, manifestBytes, err := exp.buildMetricsPayload(swcMetrics)
				if err != nil {
					exp.logger.Error("failed to marshal transformed metrics", zap.Error(err))
					continue
				}

				if err := exp.sendData(data, manifestBytes, metrics); err != nil {
					exp.logger.Error("failed to send transformed metrics", zap.Error(err))
					continue
				}
			}
		}
	}
	return nil
}

func (exp *ibmsoftwarecentralexporter) buildMetricsPayload(swcMetrics swcAccountMetricsV1.ReportSlice) ([]byte, []byte, error) {
	data, err := json.Marshal(swcMetrics)
	if err != nil {
		return nil, nil, err
	}
	manifest := dataReporterV1.Manifest{Type: "metricsReporter", Version: "1"}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, err
	}
	exp.logger.Debug("metrics data payload", zap.String("data", string(data)))
	return data, manifestBytes, nil
}

func validate(metric pmetric.Metric) []string {
	var errs []string
	switch metric.Type() {
	case pmetric.MetricTypeGauge, pmetric.MetricTypeSum:
		// Supported types.
	default:
		errs = append(errs, fmt.Sprintf("unsupported metric type: %s", metric.Type().String()))
		return errs
	}
	errs = append(errs, validateReportDataFields(metric)...)
	errs = append(errs, validateMeasuredUsage(metric)...)
	return errs
}

func validateReportDataFields(metric pmetric.Metric) []string {
	var errs []string
	var attrs pcommon.Map
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		if metric.Gauge().DataPoints().Len() > 0 {
			attrs = metric.Gauge().DataPoints().At(0).Attributes()
		} else {
			errs = append(errs, "no gauge datapoints available")
			return errs
		}
	case pmetric.MetricTypeSum:
		if metric.Sum().DataPoints().Len() > 0 {
			attrs = metric.Sum().DataPoints().At(0).Attributes()
		} else {
			errs = append(errs, "no sum datapoints available")
			return errs
		}
	}

	if getAttribute(attrs, "eventId") == "" {
		errs = append(errs, "missing field: eventId")
	}
	if getAttributeInt64(attrs, "start", 0) == 0 {
		errs = append(errs, "missing field: start")
	}
	if getAttributeInt64(attrs, "end", 0) == 0 {
		errs = append(errs, "missing field: end")
	}
	if getAttribute(attrs, "accountId") == "" {
		errs = append(errs, "missing field: accountId")
	}

	return errs
}

func validateMeasuredUsage(metric pmetric.Metric) []string {
	var errs []string
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		if metric.Gauge().DataPoints().Len() == 0 {
			errs = append(errs, "no valid measured usage datapoints available: at least one datapoint must include both a non-empty 'metricId' and a valid 'value'")
		} else {
			for i := 0; i < metric.Gauge().DataPoints().Len(); i++ {
				dp := metric.Gauge().DataPoints().At(i)
				dpErrs := validateMeasuredUsageDataPoint(dp)
				if len(dpErrs) > 0 {
					errs = append(errs, fmt.Sprintf("Gauge data point %d missing: %v", i, dpErrs))
				}
			}
		}
	case pmetric.MetricTypeSum:
		if metric.Sum().DataPoints().Len() == 0 {
			errs = append(errs, "no valid measured usage datapoints available: at least one datapoint must include both a non-empty 'metricId' and a valid 'value'")
		} else {
			for i := 0; i < metric.Sum().DataPoints().Len(); i++ {
				dp := metric.Sum().DataPoints().At(i)
				dpErrs := validateMeasuredUsageDataPoint(dp)
				if len(dpErrs) > 0 {
					errs = append(errs, fmt.Sprintf("Sum data point %d missing: %v", i, dpErrs))
				}
			}
		}
	}
	return errs
}

func validateMeasuredUsageDataPoint(dp pmetric.NumberDataPoint) []string {
	var errs []string
	attrs := dp.Attributes()
	if getAttribute(attrs, "metricId") == "" {
		errs = append(errs, "missing field: metricId")
	}
	if _, ok := attrs.Get("value"); !ok {
		errs = append(errs, "missing field: value")
	}
	return errs
}

func transformMetrics(metric pmetric.Metric) swcAccountMetricsV1.ReportSlice {
	reportData := transformReportData(metric)
	// We assume validation has passed, so reportData is not nil.
	reportData.MeasuredUsage = transformMeasuredUsage(metric)
	return swcAccountMetricsV1.ReportSlice{
		Metadata: buildSourceMetadata(metric),
		Metrics:  []*swcAccountMetricsV1.ReportData{reportData},
	}
}

func buildSourceMetadata(metric pmetric.Metric) *swcAccountMetricsV1.SourceMetadata {
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
	return &swcAccountMetricsV1.SourceMetadata{
		ClusterID:     getAttribute(attrs, "clusterId"),
		AccountID:     getAttribute(attrs, "accountId"),
		Version:       getAttribute(attrs, "version"),
		ReportVersion: getAttribute(attrs, "reportVersion"),
		Environment:   getReportEnvironment(getAttribute(attrs, "environment")),
	}
}

func transformReportData(metric pmetric.Metric) *swcAccountMetricsV1.ReportData {
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
	return &swcAccountMetricsV1.ReportData{
		EventID:                        getAttribute(attrs, "eventId"),
		IntervalStart:                  getAttributeInt64(attrs, "start", 0),
		IntervalEnd:                    getAttributeInt64(attrs, "end", 0),
		AccountID:                      getAttribute(attrs, "accountId"),
		SubscriptionId:                 getAttribute(attrs, "subscriptionId"),
		Source:                         getAttribute(attrs, "source"),
		SourceSaas:                     getAttribute(attrs, "sourceSaas"),
		AccountIdSaas:                  getAttribute(attrs, "accountIdSaas"),
		SubscriptionIdSaas:             getAttribute(attrs, "subscriptionIdSaas"),
		ProductType:                    getAttribute(attrs, "productType"),
		LicensePartNumber:              getAttribute(attrs, "licensePartNumber"),
		ProductId:                      getAttribute(attrs, "productId"),
		SapEntitlementLine:             getAttribute(attrs, "sapEntitlementLine"),
		ProductName:                    getAttribute(attrs, "productName"),
		ParentProductId:                getAttribute(attrs, "parentProductId"),
		ParentProductName:              getAttribute(attrs, "parentProductName"),
		ParentMetricId:                 getAttribute(attrs, "parentMetricId"),
		TopLevelProductId:              getAttribute(attrs, "topLevelProductId"),
		TopLevelProductName:            getAttribute(attrs, "topLevelProductName"),
		TopLevelProductMetricId:        getAttribute(attrs, "topLevelProductMetricId"),
		DswOfferAccountingSystemCode:   getAttribute(attrs, "dswOfferAccountingSystemCode"),
		DswSubscriptionAgreementNumber: getAttribute(attrs, "dswSubscriptionAgreementNumber"),
		SsmSubscriptionId:              getAttribute(attrs, "ssmSubscriptionId"),
		ICN:                            getAttribute(attrs, "ICN"),
		Group:                          getAttribute(attrs, "group"),
		GroupName:                      getAttribute(attrs, "groupName"),
		Kind:                           getAttribute(attrs, "kind"),
	}
}

func transformMeasuredUsage(metric pmetric.Metric) []swcAccountMetricsV1.MeasuredUsage {
	var usageList []swcAccountMetricsV1.MeasuredUsage
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		g := metric.Gauge()
		for i := 0; i < g.DataPoints().Len(); i++ {
			dp := g.DataPoints().At(i)
			usage := swcAccountMetricsV1.MeasuredUsage{
				MetricID:               getAttribute(dp.Attributes(), "metricId"),
				Value:                  getFloat64FromAttribute(dp.Attributes(), "value", dp.DoubleValue()),
				MeterDefNamespace:      getAttribute(dp.Attributes(), "meter_def_namespace"),
				MeterDefName:           getAttribute(dp.Attributes(), "meter_def_name"),
				MetricType:             getAttribute(dp.Attributes(), "metricType"),
				MetricAggregationType:  getAttribute(dp.Attributes(), "metricAggregationType"),
				MeasuredMetricId:       getAttribute(dp.Attributes(), "measuredMetricId"),
				ProductConversionRatio: getAttribute(dp.Attributes(), "productConversionRatio"),
				MeasuredValue:          getAttribute(dp.Attributes(), "measuredValue"),
				ClusterId:              getAttribute(dp.Attributes(), "clusterId"),
				Hostname:               getAttribute(dp.Attributes(), "hostname"),
				Pod:                    getAttribute(dp.Attributes(), "pod"),
				PlatformId:             getAttribute(dp.Attributes(), "platformId"),
				Crn:                    getAttribute(dp.Attributes(), "crn"),
				IsViewable:             getAttribute(dp.Attributes(), "isViewable"),
				CalculateSummary:       getAttribute(dp.Attributes(), "calculateSummary"),
			}
			usageList = append(usageList, usage)
		}
	case pmetric.MetricTypeSum:
		s := metric.Sum()
		for i := 0; i < s.DataPoints().Len(); i++ {
			dp := s.DataPoints().At(i)
			usage := swcAccountMetricsV1.MeasuredUsage{
				MetricID:               getAttribute(dp.Attributes(), "metricId"),
				Value:                  getFloat64FromAttribute(dp.Attributes(), "value", dp.DoubleValue()),
				MeterDefNamespace:      getAttribute(dp.Attributes(), "meter_def_namespace"),
				MeterDefName:           getAttribute(dp.Attributes(), "meter_def_name"),
				MetricType:             getAttribute(dp.Attributes(), "metricType"),
				MetricAggregationType:  getAttribute(dp.Attributes(), "metricAggregationType"),
				MeasuredMetricId:       getAttribute(dp.Attributes(), "measuredMetricId"),
				ProductConversionRatio: getAttribute(dp.Attributes(), "productConversionRatio"),
				MeasuredValue:          getAttribute(dp.Attributes(), "measuredValue"),
				ClusterId:              getAttribute(dp.Attributes(), "clusterId"),
				Hostname:               getAttribute(dp.Attributes(), "hostname"),
				Pod:                    getAttribute(dp.Attributes(), "pod"),
				PlatformId:             getAttribute(dp.Attributes(), "platformId"),
				Crn:                    getAttribute(dp.Attributes(), "crn"),
				IsViewable:             getAttribute(dp.Attributes(), "isViewable"),
				CalculateSummary:       getAttribute(dp.Attributes(), "calculateSummary"),
			}
			usageList = append(usageList, usage)
		}
	}
	return usageList
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

func getAttribute(attrs pcommon.Map, key string) string {
	if val, ok := attrs.Get(key); ok {
		return val.AsString()
	}
	return ""
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

func getReportEnvironment(environmentStr string) swcAccountMetricsV1.ReportEnvironment {
	switch environmentStr {
	case string(swcAccountMetricsV1.ReportProductionEnv):
		return swcAccountMetricsV1.ReportProductionEnv
	case string(swcAccountMetricsV1.ReportSandboxEnv):
		return swcAccountMetricsV1.ReportSandboxEnv
	default:
		return ""
	}
}

func logTransformedMetrics(logger *zap.Logger, swcMetrics swcAccountMetricsV1.ReportSlice) {
	b, err := json.MarshalIndent(swcMetrics, "", "  ")
	if err != nil {
		logger.Debug("error marshalling metrics", zap.Error(err))
		return
	}

	logger.Debug("Transformed SWC Account Metrics:\n" + string(b))
}
