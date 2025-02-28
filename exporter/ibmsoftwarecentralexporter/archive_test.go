// Copyright 2025 IBM Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ibmsoftwarecentralexporter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"reflect"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	gp "github.com/ungerik/go-pool"
)

type nopCloser struct {
	io.Writer
}

func (n nopCloser) Close() error { return nil }

func newDummyGzipPool(newFunc func() interface{}) gp.GzipPool {
	var p gp.GzipPool
	// Override the unexported field "writers".
	v := reflect.ValueOf(&p).Elem().FieldByName("writers")
	if !v.IsValid() {
		panic("writers field not found")
	}
	ptr := unsafe.Pointer(v.UnsafeAddr())
	reflect.NewAt(v.Type(), ptr).Elem().Set(reflect.ValueOf(sync.Pool{New: newFunc}))
	return p
}

func extractArchiveFiles(archive []byte) (map[string]string, error) {
	result := make(map[string]string)
	buf := bytes.NewBuffer(archive)
	gzr, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		fBuf := new(bytes.Buffer)
		if _, err := io.Copy(fBuf, tr); err != nil {
			return nil, err
		}
		result[header.Name] = fBuf.String()
	}
	return result, nil
}

func TestTGZSuccess(t *testing.T) {
	// Use a dummy GzipPool that returns a real gzip.Writer.
	pool := &TarGzipPool{
		GzipPool: newDummyGzipPool(func() interface{} {
			// Return a new gzip.Writer wrapping a nopCloser over a bytes.Buffer.
			// (The underlying writer is ignored because TGZ() passes in its own buffer.)
			return gzip.NewWriter(nopCloser{&bytes.Buffer{}})
		}),
	}
	manifest := []byte(`{"manifest":"data"}`)
	data := []byte(`{"events":"data"}`)
	result, err := pool.TGZ("testuuid", manifest, data)
	assert.NoError(t, err)
	// Decompress and extract files from archive.
	files, err := extractArchiveFiles(result)
	assert.NoError(t, err)
	assert.Contains(t, files, "manifest.json")
	assert.Contains(t, files, "testuuid.json")
	assert.Contains(t, files["manifest.json"], string(manifest))
	assert.Contains(t, files["testuuid.json"], string(data))
}
