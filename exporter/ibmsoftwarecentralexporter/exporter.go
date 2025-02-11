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
	"mime/multipart"
	"net/http"
	"net/textproto"

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

type ibmsoftwarecentralexporter struct {
	config            *Config
	telemetrySettings component.TelemetrySettings
	logger            *zap.Logger
	client            *http.Client
	tarGzipPool       *TarGzipPool
}

type swcAccountMetricsExporter struct {
	config            *Config
	telemetrySettings component.TelemetrySettings
	logger            *zap.Logger
	client            *http.Client
	tarGzipPool       *TarGzipPool
}

func initExporter(cfg *Config, createSettings exporter.Settings) (*ibmsoftwarecentralexporter, error) {
	tarGzipPool := &TarGzipPool{}

	se := &ibmsoftwarecentralexporter{
		config:            cfg,
		telemetrySettings: createSettings.TelemetrySettings,
		logger:            createSettings.Logger,
		tarGzipPool:       tarGzipPool,
	}

	se.logger.Info("IBM Software Central Exporter configured",
		zap.String("endpoint", cfg.Endpoint),
	)

	return se, nil
}

func initSWCAccountMetricsExporter(cfg *Config, createSettings exporter.Settings) (*swcAccountMetricsExporter, error) {
	tarGzipPool := &TarGzipPool{}

	se := &swcAccountMetricsExporter{
		config:            cfg,
		telemetrySettings: createSettings.TelemetrySettings,
		logger:            createSettings.Logger,
		tarGzipPool:       tarGzipPool,
	}

	se.logger.Info("SWC Account Metrics Exporter configured",
		zap.String("endpoint", cfg.Endpoint),
	)

	return se, nil
}

func newLogsExporter(
	ctx context.Context,
	params exporter.Settings,
	cfg *Config,
) (exporter.Logs, error) {
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

func newMetricsExporter(
	ctx context.Context,
	params exporter.Settings,
	cfg *Config,
) (exporter.Metrics, error) {
	s, err := initSWCAccountMetricsExporter(cfg, params)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the metrics exporter: %w", err)
	}

	return exporterhelper.NewMetrics(ctx, params, cfg, s.pushMetricsData)
}

func (se *ibmsoftwarecentralexporter) start(ctx context.Context, host component.Host) (err error) {
	if se.config.ClientConfig.Auth != nil {
		se.logger.Debug("auth",
			zap.String("clientConfig.Auth", fmt.Sprint(se.config.ClientConfig.Auth)),
		)
	} else {
		se.logger.Debug("auth",
			zap.String("clientConfig.Auth", "isNil"),
		)
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
				se.logger.Debug("logRecord",
					zap.String("logRecord.Body().AsString()", logRecord.Body().AsString()),
				)
				b := []byte(logRecord.Body().AsString())
				if json.Valid(b) {
					eventJsons = append(eventJsons, b)
				} else {
					se.logger.Debug("logRecord was not json",
						zap.String("logRecord.Body().AsString()", logRecord.Body().AsString()),
					)
				}
			}
		}
	}

	if len(eventJsons) > 0 {
		metadata := make(Metadata)
		reportData := ReportData{Metadata: metadata, EventJsons: eventJsons}
		reportDataBytes, err := json.Marshal(reportData)
		if err != nil {
			return err
		}

		se.logger.Debug("report",
			zap.String("data", string(reportDataBytes)),
		)

		manifest := Manifest{Type: "dataReporter", Version: "1"}
		manifestBytes, err := json.Marshal(manifest)
		if err != nil {
			return err
		}

		se.logger.Debug("report",
			zap.String("manifest", string(manifestBytes)),
		)

		id := uuid.New()

		archive, err := se.tarGzipPool.TGZ(id.String(), manifestBytes, reportDataBytes)
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

		err = writer.Close()
		if err != nil {
			return err
		}

		req, err := http.NewRequest(http.MethodPost, se.config.Endpoint, reqBody)
		if err != nil {
			return consumererror.NewLogs(err, logs)
		}
		for k, v := range se.config.ClientConfig.Headers {
			req.Header.Set(k, string(v))
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		res, err := se.client.Do(req)
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
		case res.StatusCode >= 200 && res.StatusCode < 300: // Success
			break
		case res.StatusCode >= 500 && res.StatusCode < 600: // Retryable error
			return rerr
		case res.StatusCode == http.StatusTooManyRequests: // Retryable error
			return rerr
		default: // Terminal error
			return consumererror.NewPermanent(fmt.Errorf("line protocol write returned %q %q", res.Status, string(resBody)))
		}
	}
	return nil

}

/*
type ConsumeMetricsFunc func(ctx context.Context, md pmetric.Metrics) error
*/
func (se *swcAccountMetricsExporter) pushMetricsData(ctx context.Context, metrics pmetric.Metrics) error {

	swcMetrics := transformMetrics(metrics)

	fmt.Printf("Transformed SWC Account Metrics: %+v\n", swcMetrics)

	return nil
}

func transformMetrics(metrics pmetric.Metrics) []v3alpha1.SourceMetadata {
	var transformedMetrics []v3alpha1.SourceMetadata

	rm := metrics.ResourceMetrics()
	for i := 0; i < rm.Len(); i++ {
		resourceMetric := rm.At(i)
		ilms := resourceMetric.ScopeMetrics()

		for j := 0; j < ilms.Len(); j++ {
			metrics := ilms.At(j).Metrics()

			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k) // unused for now I don't have the full schema here

				clusterID, accountID, version := extractMetricAttributes(resourceMetric)

				swcMetric := v3alpha1.SourceMetadata{
					ClusterID:     clusterID,
					AccountID:     accountID,
					Environment:   v3alpha1.ReportEnvironment("report environment"),
					Version:       version,
					ReportVersion: "v1",
				}

				fmt.Printf("Processing Metric: %s\n", metric.Name())

				transformedMetrics = append(transformedMetrics, swcMetric)
			}
		}
	}

	return transformedMetrics
}

func extractMetricAttributes(rm pmetric.ResourceMetrics) (string, string, string) {
	attrs := rm.Resource().Attributes()
	clusterID := getAttribute(attrs, "clusterId")
	accountID := getAttribute(attrs, "accountId")
	version := getAttribute(attrs, "version")

	return clusterID, accountID, version
}

func getAttribute(attrs pcommon.Map, key string) string {
	val, ok := attrs.Get(key)
	if ok {
		return val.AsString()
	}
	return ""
}
