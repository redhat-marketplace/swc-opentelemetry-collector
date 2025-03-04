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

import "encoding/json"

type EventJsons []json.RawMessage

type Metadata map[string]string

type Manifest struct {
	Type    string `json:"type,omitempty"`
	Version string `json:"version,omitempty"`
}

type ReportData struct {
	Metadata   Metadata          `json:"metadata,omitempty"`
	EventJsons []json.RawMessage `json:"data"`
}
