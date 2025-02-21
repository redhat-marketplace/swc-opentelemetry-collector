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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"

	// "go.opentelemetry.io/collector/config/config"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

type dummyTarGzipPool TarGzipPool

func (d *dummyTarGzipPool) TGZ(id string, manifestBytes, payloadBytes []byte) ([]byte, error) {
	return append(manifestBytes, payloadBytes...), nil
}

// createTestGaugeMetric creates a test gauge metric with one datapoint.
// If valid is true, it sets required resource attributes on both the resource and the datapoint.
func createTestGaugeMetric(valid bool) pmetric.Metric {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	if valid {
		rm.Resource().Attributes().PutStr("eventId", "testEvent")
		rm.Resource().Attributes().PutStr("accountId", "testAccount")
		rm.Resource().Attributes().PutInt("start", 1630000000000)
		rm.Resource().Attributes().PutInt("end", 1630003600000)
	}
	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("testMetric")
	gauge := metric.SetEmptyGauge()
	dp := gauge.DataPoints().AppendEmpty()
	dp.SetDoubleValue(42)
	// Set measured usage required fields.
	dp.Attributes().PutStr("metricId", "testMetricID")
	dp.Attributes().PutStr("value", "42")
	dp.Attributes().PutStr("meter_def_namespace", "ns")
	dp.Attributes().PutStr("meter_def_name", "meter")
	if valid {
		dp.Attributes().PutStr("eventId", "testEvent")
		dp.Attributes().PutStr("accountId", "testAccount")
		dp.Attributes().PutInt("start", 1630000000000)
		dp.Attributes().PutInt("end", 1630003600000)
	}
	return metric
}

func TestValidate_Success(t *testing.T) {
	metric := createTestGaugeMetric(true)
	valid := validate(metric)
	assert.True(t, valid, "expected metric to pass validation")
}

func TestValidate_Failure(t *testing.T) {
	metric := createTestGaugeMetric(false)
	valid := validate(metric)
	assert.False(t, valid, "expected metric to fail validation")
}

func TestTransformMetrics(t *testing.T) {
	metric := createTestGaugeMetric(true)
	transformed := transformMetrics(metric)
	assert.NotNil(t, transformed)
	assert.NotEmpty(t, transformed.Metrics)
	for _, md := range transformed.Metrics {
		assert.NotEmpty(t, md.EventID, "EventID should not be empty")
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

	// Build dummy metrics.
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("eventId", "testEvent")
	rm.Resource().Attributes().PutStr("accountId", "testAccount")
	rm.Resource().Attributes().PutInt("start", 1630000000000)
	rm.Resource().Attributes().PutInt("end", 1630003600000)
	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("testMetric")
	gauge := metric.SetEmptyGauge()
	dp := gauge.DataPoints().AppendEmpty()
	dp.SetDoubleValue(42)
	dp.Attributes().PutStr("metricId", "testMetricID")
	dp.Attributes().PutStr("value", "42")
	dp.Attributes().PutStr("meter_def_namespace", "ns")
	dp.Attributes().PutStr("meter_def_name", "meter")
	dp.Attributes().PutStr("eventId", "testEvent")
	dp.Attributes().PutStr("accountId", "testAccount")
	dp.Attributes().PutInt("start", 1630000000000)
	dp.Attributes().PutInt("end", 1630003600000)

	// Setup a test HTTP server that returns 200.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
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
			// Headers:  map[string]config.Value{"Test-Header": config.NewValueStr("value")},
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
			Endpoint: "http://[::1", // Invalid URL
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
			Endpoint: "http://[::1", // Invalid URL
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
