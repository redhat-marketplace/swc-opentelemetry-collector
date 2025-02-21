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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/config/confighttp"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "invalid endpoint - bad escape",
			cfg: &Config{
				ClientConfig: confighttp.ClientConfig{
					Endpoint: "http://bad\\escape",
				},
			},
			wantErr: true,
			errMsg:  "invalid endpoint: endpoint is required but it is not configured",
		},
		{
			name: "empty endpoint",
			cfg: &Config{
				ClientConfig: confighttp.ClientConfig{
					Endpoint: "",
				},
			},
			wantErr: true,
			errMsg:  "invalid endpoint: endpoint is required but it is not configured",
		},
		{
			name: "valid endpoint",
			cfg: &Config{
				ClientConfig: confighttp.ClientConfig{
					Endpoint: DefaultEndpoint,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
