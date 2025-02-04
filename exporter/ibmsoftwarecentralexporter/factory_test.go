// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package ibmsoftwarecentralexporter

import (
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter/internal/metadata"
)

func TestType(t *testing.T) {
	factory := NewFactory()
	pType := factory.Type()
	assert.Equal(t, pType, metadata.Type)
}

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig()

	assert.Equal(t, &Config{
		Endpoint: "https://swc.saas.ibm.com/metering/api/v2/metrics",
		QueueSettings: exporterhelper.QueueConfig{
			Enabled:      false,
			NumConsumers: 10,
			QueueSize:    1000,
		},
		BackOffConfig: configretry.BackOffConfig{
			Enabled:             true,
			InitialInterval:     5 * time.Second,
			RandomizationFactor: backoff.DefaultRandomizationFactor,
			Multiplier:          backoff.DefaultMultiplier,
			MaxInterval:         30 * time.Second,
			MaxElapsedTime:      5 * time.Minute,
		},
		TimeoutSettings: exporterhelper.TimeoutConfig{
			Timeout: 5 * time.Second,
		},
	}, cfg)
}
