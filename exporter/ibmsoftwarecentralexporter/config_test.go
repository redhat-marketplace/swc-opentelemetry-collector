// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package ibmsoftwarecentralexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		err  string
	}{
		{
			name: "invalid Endpoint",
			cfg: &Config{
				Endpoint: "http://bad\\escape",
			},
			err: "invalid endpoint: endpoint is required but it is not configured",
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			err := testInstance.cfg.Validate()
			if testInstance.err != "" {
				assert.EqualError(t, err, testInstance.err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
