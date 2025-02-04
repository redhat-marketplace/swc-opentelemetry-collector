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
