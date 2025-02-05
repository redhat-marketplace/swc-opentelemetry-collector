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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

type ibmsoftwarecentralexporter struct {
	config            *Config
	telemetrySettings component.TelemetrySettings
	logger            *zap.Logger
	tlsConfig         *tls.Config
	client            *http.Client
	tarGzipPool       *TarGzipPool
}

func initExporter(cfg *Config, createSettings exporter.Settings) (*ibmsoftwarecentralexporter, error) {
	var loadedTLSConfig *tls.Config
	loadedTLSConfig, err := cfg.TLSSetting.LoadTLSConfig(context.Background())
	if err != nil {
		return nil, err
	}

	tarGzipPool := &TarGzipPool{}

	i := &ibmsoftwarecentralexporter{
		config:            cfg,
		telemetrySettings: createSettings.TelemetrySettings,
		logger:            createSettings.Logger,
		tlsConfig:         loadedTLSConfig,
		tarGzipPool:       tarGzipPool,
	}

	i.logger.Info("IBM Software Central Exporter configured",
		zap.String("endpoint", cfg.Endpoint),
	)

	return i, nil
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

func (se *ibmsoftwarecentralexporter) start(ctx context.Context, host component.Host) (err error) {
	clientConfig := confighttp.NewDefaultClientConfig()
	clientConfig.Timeout = 30 * time.Second
	se.client, err = clientConfig.ToClient(ctx, host, se.telemetrySettings)
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
				if json.Valid(logRecord.Body().Bytes().AsRaw()) {
					eventJsons = append(eventJsons, logRecord.Body().Bytes().AsRaw())
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

		manifest := Manifest{Type: "dataReporter", Version: "1"}
		manifestBytes, err := json.Marshal(manifest)
		if err != nil {
			return err
		}

		id := uuid.New()

		archive, err := se.tarGzipPool.TGZ(id.String(), manifestBytes, reportDataBytes)
		if err != nil {
			return err
		}

		req, err := http.NewRequest(http.MethodPost, se.config.Endpoint, bytes.NewBuffer(archive))
		if err != nil {
			return consumererror.NewLogs(err, logs)
		}

		res, err := se.client.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		if err = res.Body.Close(); err != nil {
			return err
		}
		rerr := fmt.Errorf("remote write returned HTTP status %v; err = %w: %s", res.Status, err, string(body))
		switch {
		case res.StatusCode >= 200 && res.StatusCode < 300: // Success
			break
		case res.StatusCode >= 500 && res.StatusCode < 600: // Retryable error
			return rerr
		case res.StatusCode == http.StatusTooManyRequests: // Retryable error
			return rerr
		default: // Terminal error
			return consumererror.NewPermanent(fmt.Errorf("line protocol write returned %q %q", res.Status, string(body)))
		}
	}
	return nil

}
