// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package ibmsoftwarecentralexporter // import "github.com/redhat-marketplace/swc-opentelemetry-collector/exporter/ibmsoftwarecentralexporter"

import (
	"errors"
	"net/url"

	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

var (
	errInvalidEndpoint = errors.New("invalid endpoint: endpoint is required but it is not configured")
)

// Config defines configuration for IBM Software Central exporter.
type Config struct {
	// Server address
	Endpoint string `mapstructure:"endpoint"`

	// TLSSetting struct exposes TLS client configuration.
	TLSSetting configtls.ClientConfig `mapstructure:"tls"`

	QueueSettings             exporterhelper.QueueConfig `mapstructure:"sending_queue"`
	configretry.BackOffConfig `mapstructure:"retry_on_failure"`
	TimeoutSettings           exporterhelper.TimeoutConfig `mapstructure:",squash"` // squash ensures fields are correctly decoded in embedded struct
}

// Validate the configuration for errors. This is required by component.Config.
func (cfg *Config) Validate() error {
	invalidFields := []error{}

	_, err := url.Parse(cfg.Endpoint)
	if err != nil {
		invalidFields = append(invalidFields, errInvalidEndpoint)
	}

	if cfg.Endpoint == "" {
		invalidFields = append(invalidFields, errInvalidEndpoint)
	}

	if len(invalidFields) > 0 {
		return errors.Join(invalidFields...)
	}

	return nil
}

const (
	DefaultEndpoint = "https://swc.saas.ibm.com/metering/api/v2/metrics"
)
