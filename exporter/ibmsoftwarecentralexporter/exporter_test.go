// exporter_test.go
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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter/v3alpha1"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

type dummyTarGzipPool TarGzipPool

func (d *dummyTarGzipPool) TGZ(id string, manifestBytes, payloadBytes []byte) ([]byte, error) {
	// For testing, simply concatenate manifest and payload.
	return append(manifestBytes, payloadBytes...), nil
}

// createTestMetric returns a pmetric.Metric with optional data points.
//   - mType: pmetric.MetricType (Gauge, Sum, or an unsupported type like Histogram).
//   - hasDataPoint: if true, appends exactly one NumberDataPoint; if false, zero datapoints.
//   - setEventFields: if true, sets the "event-level" fields (eventId, accountId, start, end)
//   - setUsageFields: if true, sets the "measured usage" fields (metricId, value, meter_def_namespace, meter_def_name)
func createTestMetric(
	mType pmetric.MetricType,
	hasDataPoint bool,
	setEventFields bool,
	setUsageFields bool,
) pmetric.Metric {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()

	switch mType {
	case pmetric.MetricTypeGauge:
		metric.SetEmptyGauge()
	case pmetric.MetricTypeSum:
		metric.SetEmptySum()
	default:
		// For unsupported types, e.g. Histogram.
		metric.SetEmptyHistogram()
	}

	// Optionally add exactly one data point (only for Gauge or Sum).
	if hasDataPoint && (mType == pmetric.MetricTypeGauge || mType == pmetric.MetricTypeSum) {
		var dp pmetric.NumberDataPoint
		if mType == pmetric.MetricTypeGauge {
			dp = metric.Gauge().DataPoints().AppendEmpty()
		} else {
			dp = metric.Sum().DataPoints().AppendEmpty()
		}
		dp.SetDoubleValue(42) // arbitrary value

		if setEventFields {
			dp.Attributes().PutStr("eventId", "testEvent")
			dp.Attributes().PutStr("accountId", "testAccount")
			dp.Attributes().PutInt("start", 1000)
			dp.Attributes().PutInt("end", 2000)
		}
		if setUsageFields {
			dp.Attributes().PutStr("metricId", "testMetricID")
			dp.Attributes().PutStr("value", "42")
			dp.Attributes().PutStr("meter_def_namespace", "ns")
			dp.Attributes().PutStr("meter_def_name", "meter")
		}
	}

	return metric
}

func TestValidateReportDataFields_Refactored(t *testing.T) {
	tests := []struct {
		name           string
		mType          pmetric.MetricType
		hasDataPoint   bool
		setEventFields bool
		expectedErrs   []string
		description    string
	}{
		{
			name:           "GaugeNoDP",
			mType:          pmetric.MetricTypeGauge,
			hasDataPoint:   false,
			setEventFields: false,
			expectedErrs:   []string{"no gauge datapoints available"},
			description:    "Gauge with zero datapoints should return an error indicating no gauge datapoints available",
		},
		{
			name:           "SumNoDP",
			mType:          pmetric.MetricTypeSum,
			hasDataPoint:   false,
			setEventFields: false,
			expectedErrs:   []string{"no sum datapoints available"},
			description:    "Sum with zero datapoints should return an error indicating no sum datapoints available",
		},
		{
			name:           "GaugeMissingEventFields",
			mType:          pmetric.MetricTypeGauge,
			hasDataPoint:   true,
			setEventFields: false,
			expectedErrs:   []string{"missing field: eventId", "missing field: start", "missing field: end", "missing field: accountId"},
			description:    "Gauge with a datapoint missing event fields should return errors",
		},
		{
			name:           "SumMissingEventFields",
			mType:          pmetric.MetricTypeSum,
			hasDataPoint:   true,
			setEventFields: false,
			expectedErrs:   []string{"missing field: eventId", "missing field: start", "missing field: end", "missing field: accountId"},
			description:    "Sum with a datapoint missing event fields should return errors",
		},
		{
			name:           "GaugeValid",
			mType:          pmetric.MetricTypeGauge,
			hasDataPoint:   true,
			setEventFields: true,
			expectedErrs:   []string{},
			description:    "Gauge with all required event fields should return an empty error slice",
		},
		{
			name:           "SumValid",
			mType:          pmetric.MetricTypeSum,
			hasDataPoint:   true,
			setEventFields: true,
			expectedErrs:   []string{},
			description:    "Sum with all required event fields should return an empty error slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := createTestMetric(tt.mType, tt.hasDataPoint, tt.setEventFields, false)
			errs := validateReportDataFields(metric)
			if len(tt.expectedErrs) == 0 {
				assert.Empty(t, errs, tt.description)
			} else {
				assert.Equal(t, tt.expectedErrs, errs, tt.description)
			}
		})
	}
}

func TestValidateMeasuredUsage_Refactored(t *testing.T) {
	tests := []struct {
		name           string
		mType          pmetric.MetricType
		hasDataPoint   bool
		setUsageFields bool
		expectedErrs   []string
		description    string
	}{
		{
			name:           "GaugeNoDP",
			mType:          pmetric.MetricTypeGauge,
			hasDataPoint:   false,
			setUsageFields: false,
			expectedErrs:   []string{"no valid measured usage datapoints available: at least one datapoint must include both a non-empty 'metricId' and a valid 'value'"},
			description:    "Gauge with zero datapoints should return error indicating no valid usage datapoints",
		},
		{
			name:           "SumNoDP",
			mType:          pmetric.MetricTypeSum,
			hasDataPoint:   false,
			setUsageFields: false,
			expectedErrs:   []string{"no valid measured usage datapoints available: at least one datapoint must include both a non-empty 'metricId' and a valid 'value'"},
			description:    "Sum with zero datapoints should return error indicating no valid usage datapoints",
		},
		{
			name:           "GaugeMissingUsageFields",
			mType:          pmetric.MetricTypeGauge,
			hasDataPoint:   true,
			setUsageFields: false,
			expectedErrs:   []string{"Gauge data point 0 missing: [missing field: metricId missing field: value]"},
			description:    "Gauge datapoint missing usage fields should return errors",
		},
		{
			name:           "SumMissingUsageFields",
			mType:          pmetric.MetricTypeSum,
			hasDataPoint:   true,
			setUsageFields: false,
			expectedErrs:   []string{"Sum data point 0 missing: [missing field: metricId missing field: value]"},
			description:    "Sum datapoint missing usage fields should return errors",
		},
		{
			name:           "GaugeValid",
			mType:          pmetric.MetricTypeGauge,
			hasDataPoint:   true,
			setUsageFields: true,
			expectedErrs:   []string{},
			description:    "Gauge with valid usage fields should return an empty error slice",
		},
		{
			name:           "SumValid",
			mType:          pmetric.MetricTypeSum,
			hasDataPoint:   true,
			setUsageFields: true,
			expectedErrs:   []string{},
			description:    "Sum with valid usage fields should return an empty error slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setEventFields is irrelevant for usage validation.
			metric := createTestMetric(tt.mType, tt.hasDataPoint, false, tt.setUsageFields)
			errs := validateMeasuredUsage(metric)
			if len(tt.expectedErrs) == 0 {
				assert.Empty(t, errs, tt.description)
			} else {
				assert.Equal(t, tt.expectedErrs, errs, tt.description)
			}
		})
	}
}

func TestValidate_Success(t *testing.T) {
	// A gauge with 1 datapoint that sets both event-level and usage fields should pass.
	gaugeMetric := createTestMetric(pmetric.MetricTypeGauge, true, true, true)
	errs := validate(gaugeMetric)
	assert.Empty(t, errs, "expected gauge metric to pass validation (no errors)")

	// A sum with 1 datapoint that sets both event-level and usage fields should pass.
	sumMetric := createTestMetric(pmetric.MetricTypeSum, true, true, true)
	errs = validate(sumMetric)
	assert.Empty(t, errs, "expected sum metric to pass validation (no errors)")
}

func TestValidate_Failure(t *testing.T) {
	// A gauge with usage fields set but missing event-level fields should fail.
	gaugeMetric := createTestMetric(pmetric.MetricTypeGauge, true, false, true)
	errs := validate(gaugeMetric)
	assert.NotEmpty(t, errs, "expected gauge metric to fail validation (missing event fields)")

	// A sum with event-level fields set but missing usage fields should fail.
	sumMetric := createTestMetric(pmetric.MetricTypeSum, true, true, false)
	errs = validate(sumMetric)
	assert.NotEmpty(t, errs, "expected sum metric to fail validation (missing usage fields)")
}

func TestTransformMetrics_Gauge(t *testing.T) {
	// Create a gauge metric with 1 datapoint that has both event-level and usage fields.
	metric := createTestMetric(pmetric.MetricTypeGauge, true, true, true)
	transformed := transformMetrics(metric)
	assert.NotNil(t, transformed)
	assert.NotEmpty(t, transformed.Metrics, "should produce at least one MarketplaceReportData")

	mrd := transformed.Metrics[0]
	assert.Equal(t, "testEvent", mrd.EventID, "should read eventId from datapoint attributes")
	assert.Len(t, mrd.MeasuredUsage, 1, "should have exactly 1 usage datapoint")
	assert.Equal(t, "testMetricID", mrd.MeasuredUsage[0].MetricID, "should read usage fields")
}

func TestTransformMetrics_Sum(t *testing.T) {
	// Create a sum metric with 1 datapoint that has both event-level and usage fields.
	metric := createTestMetric(pmetric.MetricTypeSum, true, true, true)
	transformed := transformMetrics(metric)
	assert.NotNil(t, transformed)
	assert.NotEmpty(t, transformed.Metrics)

	mrd := transformed.Metrics[0]
	assert.Equal(t, "testEvent", mrd.EventID, "should read eventId from datapoint attributes")
	assert.Len(t, mrd.MeasuredUsage, 1, "should have exactly 1 usage datapoint")
	assert.Equal(t, "testMetricID", mrd.MeasuredUsage[0].MetricID, "should read usage fields")
}

func TestTransformMetrics_NoEventFields(t *testing.T) {
	// Create a gauge metric that omits event-level fields but sets usage fields.
	metric := createTestMetric(pmetric.MetricTypeGauge, true, false, true)
	result := transformMetrics(metric)
	// When event fields are missing, transformMarketplaceReportData produces a MarketplaceReportData with empty EventID.
	if len(result.Metrics) > 0 {
		mrd := result.Metrics[0]
		assert.Equal(t, "", mrd.EventID, "expected empty eventId when required event fields are missing")
	} else {
		assert.Empty(t, result.Metrics, "expected empty slice if missing required event fields")
	}
}

func TestPushMetrics(t *testing.T) {
	cfg := &Config{
		ClientConfig: confighttp.ClientConfig{
			Endpoint: "http://dummy",
		},
	}
	params := exporter.Settings{
		TelemetrySettings: component.TelemetrySettings{Logger: zap.NewNop()},
	}
	exporterInstance, err := initSWCAccountMetricsExporter(cfg, params)
	assert.NoError(t, err)

	// Build a metric with 1 datapoint that sets both event-level and usage fields.
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	metric := createTestMetric(pmetric.MetricTypeGauge, true, true, true)
	metric.CopyTo(sm.Metrics().AppendEmpty()) // Copy to the pipeline

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	}))
	defer ts.Close()

	cfg.ClientConfig.Endpoint = ts.URL
	exporterInstance.client = ts.Client()
	exporterInstance.tarGzipPool = (*TarGzipPool)(&dummyTarGzipPool{})

	err = exporterInstance.pushMetrics(context.Background(), metrics)
	assert.NoError(t, err)
}

func TestPushLogsData(t *testing.T) {
	cfg := &Config{
		ClientConfig: confighttp.ClientConfig{
			Endpoint: "http://dummy",
		},
	}
	params := exporter.Settings{
		TelemetrySettings: component.TelemetrySettings{Logger: zap.NewNop()},
	}
	exporterInstance, err := initExporter(cfg, params)
	assert.NoError(t, err)

	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.Body().SetStr(`{"key":"value"}`)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "multipart/form-data") {
			t.Errorf("Expected multipart/form-data, got %s", ct)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	cfg.ClientConfig.Endpoint = ts.URL
	exporterInstance.client = ts.Client()
	exporterInstance.tarGzipPool = (*TarGzipPool)(&dummyTarGzipPool{})

	err = exporterInstance.pushLogsData(context.Background(), logs)
	assert.NoError(t, err)
}

func TestSendData_Success(t *testing.T) {
	cfg := &Config{
		ClientConfig: confighttp.ClientConfig{
			Endpoint: "http://dummy",
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	cfg.ClientConfig.Endpoint = ts.URL
	logger := zap.NewNop()
	be := baseExporter{
		config:            cfg,
		telemetrySettings: component.TelemetrySettings{Logger: logger},
		logger:            logger,
		client:            ts.Client(),
		tarGzipPool:       (*TarGzipPool)(&dummyTarGzipPool{}),
	}

	payload := []byte("payload")
	manifest := []byte("manifest")
	err := be.sendData(payload, manifest, plog.NewLogs())
	assert.NoError(t, err)
}

func TestSendData_Retryable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer ts.Close()

	cfg := &Config{
		ClientConfig: confighttp.ClientConfig{
			Endpoint: ts.URL,
		},
	}
	logger := zap.NewNop()
	be := baseExporter{
		config:            cfg,
		telemetrySettings: component.TelemetrySettings{Logger: logger},
		logger:            logger,
		client:            ts.Client(),
		tarGzipPool:       (*TarGzipPool)(&dummyTarGzipPool{}),
	}

	err := be.sendData([]byte("payload"), []byte("manifest"), plog.NewLogs())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remote write returned HTTP status")
}

func TestSendData_Terminal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}))
	defer ts.Close()

	cfg := &Config{
		ClientConfig: confighttp.ClientConfig{
			Endpoint: ts.URL,
		},
	}
	logger := zap.NewNop()
	be := baseExporter{
		config:            cfg,
		telemetrySettings: component.TelemetrySettings{Logger: logger},
		logger:            logger,
		client:            ts.Client(),
		tarGzipPool:       (*TarGzipPool)(&dummyTarGzipPool{}),
	}

	err := be.sendData([]byte("payload"), []byte("manifest"), plog.NewLogs())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "line protocol write returned")
}

func TestSendData_NewRequestError_Logs(t *testing.T) {
	cfg := &Config{
		ClientConfig: confighttp.ClientConfig{
			Endpoint: "http://[::1", // invalid URL
		},
	}
	logger := zap.NewNop()
	be := baseExporter{
		config:            cfg,
		telemetrySettings: component.TelemetrySettings{Logger: logger},
		logger:            logger,
		client:            &http.Client{},
		tarGzipPool:       (*TarGzipPool)(&dummyTarGzipPool{}),
	}
	err := be.sendData([]byte("payload"), []byte("manifest"), plog.NewLogs())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing ']' in host")
}

func TestSendData_NewRequestError_Metrics(t *testing.T) {
	cfg := &Config{
		ClientConfig: confighttp.ClientConfig{
			Endpoint: "http://[::1", // invalid URL
		},
	}
	logger := zap.NewNop()
	be := baseExporter{
		config:            cfg,
		telemetrySettings: component.TelemetrySettings{Logger: logger},
		logger:            logger,
		client:            &http.Client{},
		tarGzipPool:       (*TarGzipPool)(&dummyTarGzipPool{}),
	}
	met := pmetric.NewMetrics()
	err := be.sendData([]byte("payload"), []byte("manifest"), met)
	assert.Error(t, err)
}

func TestGetDPAttribute(t *testing.T) {
	m := pcommon.NewMap()
	m.PutStr("key", "value")
	result := getDPAttribute(m, "key", "default")
	assert.Equal(t, "value", result, "should return the value for existing key")

	result = getDPAttribute(m, "missing", "default")
	assert.Equal(t, "default", result, "should return the default for missing key")
}

func TestGetAttribute(t *testing.T) {
	m := pcommon.NewMap()
	m.PutStr("key", "value")
	result := getAttribute(m, "key", "default")
	assert.Equal(t, "value", result, "should return the value for existing key")

	result = getAttribute(m, "missing", "default")
	assert.Equal(t, "default", result, "should return default when key is missing")
}

func TestGetFloat64FromAttribute(t *testing.T) {
	m := pcommon.NewMap()

	m.PutDouble("doubleKey", 3.14)
	result := getFloat64FromAttribute(m, "doubleKey", 0)
	assert.Equal(t, 3.14, result, "should return the double value")

	m.PutInt("intKey", 42)
	result = getFloat64FromAttribute(m, "intKey", 0)
	assert.Equal(t, 42.0, result, "should convert int to float64")

	m.PutStr("strKey", "2.718")
	result = getFloat64FromAttribute(m, "strKey", 0)
	expected, _ := strconv.ParseFloat("2.718", 64)
	assert.Equal(t, expected, result, "should parse string to float64")

	m.PutStr("badStr", "not_a_number")
	result = getFloat64FromAttribute(m, "badStr", 1.23)
	assert.Equal(t, 1.23, result, "should return default value on conversion error")

	result = getFloat64FromAttribute(m, "missing", 5.55)
	assert.Equal(t, 5.55, result, "should return default when attribute missing")
}

func TestGetAttributeInt64(t *testing.T) {
	m := pcommon.NewMap()

	m.PutInt("intKey", 100)
	result := getAttributeInt64(m, "intKey", 0)
	assert.Equal(t, int64(100), result, "should return the int value as int64")

	m.PutDouble("doubleKey", 200.0)
	result = getAttributeInt64(m, "doubleKey", 0)
	assert.Equal(t, int64(200), result, "should convert double to int64")

	m.PutStr("strKey", "300")
	result = getAttributeInt64(m, "strKey", 0)
	assert.Equal(t, int64(300), result, "should parse string to int64")

	m.PutStr("badStr", "abc")
	result = getAttributeInt64(m, "badStr", 400)
	assert.Equal(t, int64(400), result, "should return default on conversion error")

	result = getAttributeInt64(m, "missing", 500)
	assert.Equal(t, int64(500), result, "should return default when attribute is missing")
}

func TestGetReportEnvironment(t *testing.T) {
	result := getReportEnvironment("production")
	assert.Equal(t, v3alpha1.ReportProductionEnv, result, "should return production environment")

	result = getReportEnvironment("stage")
	assert.Equal(t, v3alpha1.ReportSandboxEnv, result, "should return sandbox environment")

	result = getReportEnvironment("unknown")
	assert.Equal(t, v3alpha1.ReportEnvironment(""), result, "should return empty for unknown environment")
}
