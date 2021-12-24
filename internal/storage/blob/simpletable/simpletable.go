/*
Maddy Mail Server - Composable all-in-one email server.
Copyright 2021, Steve Blinch <dev@blinch.ca>, Max Mazurov <fox.cpp@disroot.org>, Maddy Mail Server contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package simpletable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/foxcpp/maddy/framework/config"
	modconfig "github.com/foxcpp/maddy/framework/config/module"
	"github.com/foxcpp/maddy/framework/log"
	"github.com/foxcpp/maddy/framework/module"
)

const modName = "storage.blob.simpletable"

type Store struct {
	instName string
	log      log.Logger

	backend module.MutableTable
}

func New(_, instName string, _, inlineArgs []string) (module.Module, error) {
	if len(inlineArgs) != 0 {
		return nil, fmt.Errorf("%s: expected 0 arguments", modName)
	}

	return &Store{
		instName: instName,
		log:      log.Logger{Name: modName, Debug: log.DefaultLogger.Debug},
	}, nil
}

var (
	errMutableTable = errors.New("storage.blob.simpletable requires a mutable table type")
)

func (s *Store) Init(cfg *config.Map) error {
	var backend module.Table
	cfg.Custom("backend", false, true, func() (interface{}, error) {
		return nil, nil
	}, modconfig.TableDirective, &backend)

	if _, err := cfg.Process(); err != nil {
		return err
	}

	var ok bool
	if s.backend, ok = backend.(module.MutableTable); !ok {
		return errMutableTable
	}

	return nil
}

func (s *Store) Name() string {
	return modName
}

func (s *Store) InstanceName() string {
	return s.instName
}

type tableblob struct {
	key     string
	w       bytes.Buffer
	backend module.MutableTable
	didSync bool
}

func (b *tableblob) Sync() error {
	// We do this in Sync instead of Close because
	// backend may not actually check the error of Close.
	// The problematic restriction is that Sync can now be called
	// only once.
	if b.didSync {
		panic("storage.blob.simpletable: Sync called twice for a blob object")
	}

	err := b.backend.SetKey(b.key, b.w.String())
	if err == nil {
		err = b.Close()
		b.didSync = true
	}
	return err
}

func (b *tableblob) Write(p []byte) (n int, err error) {
	return b.w.Write(p)
}

func (b *tableblob) Close() error {
	if !b.didSync {
		return fmt.Errorf("storage.blob.simpletable: blob closed without Sync")
	}
	return nil
}

func (s *Store) Create(ctx context.Context, key string, blobSize int64) (module.Blob, error) {
	buf := bytes.Buffer{}

	if blobSize == module.UnknownBlobSize {
		blobSize = 512 * 1024
	}
	buf.Grow(int(blobSize))
	return &tableblob{
		w:       buf,
		key:     key,
		backend: s.backend,
	}, nil
}

var ErrNotExist = errors.New("requested key does not exist")

func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	value, ok, err := s.backend.Lookup(ctx, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotExist
	}

	return io.NopCloser(strings.NewReader(value)), nil
}

func (s *Store) Delete(ctx context.Context, keys []string) error {
	var lastErr error
	for _, k := range keys {
		lastErr = s.backend.RemoveKey(k)
		if lastErr != nil {
			s.log.Error("failed to delete object", lastErr, k)
		}
	}
	return lastErr
}

func init() {
	var _ module.BlobStore = &Store{}
	module.Register(modName, New)
}
