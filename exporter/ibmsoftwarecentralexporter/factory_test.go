// Copyright 2025 IBM Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package ibmsoftwarecentralexporter

import (
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"

	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"go.opentelemetry.io/collector/config/configcompression"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configretry"

	"github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter/internal/metadata"
)

func pointerToInt(v int) *int {
	return &v
}
func pointerToDuration(d time.Duration) *time.Duration {
	return &d
}

func TestType(t *testing.T) {
	factory := NewFactory()
	pType := factory.Type()
	assert.Equal(t, pType, metadata.Type)
}

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig()

	assert.Equal(t, &Config{
		ClientConfig: confighttp.ClientConfig{
			// Endpoint:             DefaultEndpoint, //TODO: do we want to always force a default in factory.go?
			Timeout:              30 * time.Second,
			Headers:              map[string]configopaque.String{},
			Compression:          "",
			CompressionParams:    configcompression.CompressionParams{Level: 0},
			Auth:                 nil,
			ProxyURL:             "",
			MaxIdleConns:         pointerToInt(100),
			MaxIdleConnsPerHost:  pointerToInt(0),
			MaxConnsPerHost:      pointerToInt(0),
			IdleConnTimeout:      pointerToDuration(90 * time.Second),
			DisableKeepAlives:    false,
			HTTP2ReadIdleTimeout: 0,
			HTTP2PingTimeout:     0,
			Cookies:              nil,
			// TLS and other fields are set by confighttp.NewDefaultClientConfig but
		},
		QueueSettings: exporterhelper.QueueConfig{
			Enabled:      false,
			NumConsumers: 10,
			QueueSize:    1000,
			Blocking:     false,
			StorageID:    nil,
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
	}, cfg, "default config should match expected values")
}
