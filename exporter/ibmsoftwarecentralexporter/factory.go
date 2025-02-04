package ibmsoftwarecentralexporter // import "github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter"

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter/internal/metadata"
)

// NewFactory returns a new factory for the ibmsoftwarecentral exporter.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		metadata.Type,
		createDefaultConfig,
		exporter.WithLogs(createLogsExporter, metadata.LogsStability),
	)
}

func createDefaultConfig() component.Config {
	qs := exporterhelper.NewDefaultQueueConfig()
	qs.Enabled = false

	return &Config{
		Endpoint:        DefaultEndpoint,
		BackOffConfig:   configretry.NewDefaultBackOffConfig(),
		QueueSettings:   qs,
		TimeoutSettings: exporterhelper.NewDefaultTimeoutConfig(),
	}
}

func createLogsExporter(
	ctx context.Context,
	params exporter.Settings,
	cfg component.Config,
) (exporter.Logs, error) {
	exp, err := newLogsExporter(ctx, params, cfg.(*Config))
	if err != nil {
		return nil, fmt.Errorf("failed to create the logs exporter: %w", err)
	}

	return exp, nil
}
