// Copyright 2023 IBM Corp.
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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"emperror.dev/errors"

	gp "github.com/ungerik/go-pool"
)

type TarGzipPool struct {
	gp.GzipPool
}

var ErrWriteSizeMismatch = errors.New("tar wrote zero or wrong size bytes")

func (pool *TarGzipPool) TGZ(uuid string, manifest []byte, data []byte) ([]byte, error) {

	var buf bytes.Buffer

	gzw := pool.GetWriter(&buf)

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// manifest
	mhdr := &tar.Header{
		Name:     "manifest.json",
		Size:     int64(len(manifest)),
		Mode:     509,
		ModTime:  time.Now(),
		Typeflag: tar.TypeReg,
	}

	err := tw.WriteHeader(mhdr)
	if err != nil {
		return nil, err
	}

	size, err := tw.Write(manifest)
	if err != nil {
		return nil, err
	}

	if size == 0 || size != len(manifest) {
		return nil, ErrWriteSizeMismatch
	}

	// events
	ehdr := &tar.Header{
		Name:     uuid + ".json",
		Size:     int64(len(data)),
		Mode:     509,
		ModTime:  time.Now(),
		Typeflag: tar.TypeReg,
	}

	err = tw.WriteHeader(ehdr)
	if err != nil {
		return nil, err
	}

	size, err = tw.Write(data)
	if err != nil {
		return nil, err
	}

	if size == 0 || size != len(data) {
		return nil, ErrWriteSizeMismatch
	}

	if twerr := tw.Close(); twerr != nil {
		err = errors.Append(err, twerr)
	}

	pool.PutWriter(gzw)

	return buf.Bytes(), err

}

func (pool *TarGzipPool) TarGzip(src string, dest string) error {

	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("unable to tar files - %v", err.Error())
	}

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	gzw := pool.GetWriter(destFile)

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	err = filepath.Walk(src, func(file string, fi os.FileInfo, errIn error) error {

		if errIn != nil {
			return errors.Wrap(errIn, "failed to tar files")
		}

		if !fi.Mode().IsRegular() {
			return nil
		}

		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return errors.Wrap(err, "failed to create new dir")
		}

		header.Name = strings.TrimPrefix(strings.ReplaceAll(file, src, ""), string(filepath.Separator))

		if err = tw.WriteHeader(header); err != nil {
			return errors.Wrap(err, "failed to write header")
		}

		f, err := os.Open(file)
		if err != nil {
			return errors.Wrap(err, "failed to open file for taring")
		}

		if _, err := io.Copy(tw, f); err != nil {
			return errors.Wrap(err, "failed to copy data")
		}

		f.Close()

		return nil
	})

	if twerr := tw.Close(); twerr != nil {
		err = errors.Append(err, twerr)
	}

	pool.PutWriter(gzw)

	if derr := destFile.Close(); derr != nil {
		err = errors.Append(err, derr)
	}

	return err
}
