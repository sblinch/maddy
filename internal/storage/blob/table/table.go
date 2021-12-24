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

package table

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/foxcpp/maddy/framework/config"
	modconfig "github.com/foxcpp/maddy/framework/config/module"
	"github.com/foxcpp/maddy/framework/log"
	"github.com/foxcpp/maddy/framework/module"
)

const modName = "storage.blob.table"

const (
	// default maximum size for a blob chunk stored in the table (and, by extension, the maximum allocation size in
	// memory); this can be overridden with the chunk_size configuration variable
	defaultChunkSize = 1 * 1024 * 1024
	// the overwhelming majority of messages are under this size, so we use smaller buffers of this size when possible
	smallBufferSize = 128 * 1024
)

type Store struct {
	instName string
	log      log.Logger

	storage   module.MutableTable
	chunkSize int64
	lgBufPool sync.Pool
	smBufPool sync.Pool
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
	errMutableTable = errors.New("storage.blob.table requires a mutable table type")
)

func (s *Store) Init(cfg *config.Map) error {
	var (
		storage module.Table
	)
	cfg.Custom("table", false, true, func() (interface{}, error) {
		return nil, nil
	}, modconfig.TableDirective, &storage)
	cfg.Int64("chunk_size", true, false, defaultChunkSize, &s.chunkSize)

	if _, err := cfg.Process(); err != nil {
		return err
	}

	var ok bool
	if s.storage, ok = storage.(module.MutableTable); !ok {
		return errMutableTable
	}

	s.lgBufPool.New = func() interface{} {
		return make([]byte, s.chunkSize)
	}
	s.smBufPool.New = func() interface{} {
		return make([]byte, smallBufferSize)
	}

	return nil
}

func (s *Store) Name() string {
	return modName
}

func (s *Store) InstanceName() string {
	return s.instName
}

type tableBlobWriter struct {
	key     string
	chunks  int
	pw      *io.PipeWriter
	storage module.MutableTable
	didSync bool
	errCh   chan error
}

func (b *tableBlobWriter) Sync() error {
	// We do this in Sync instead of Close because
	// backend may not actually check the error of Close.
	// The problematic restriction is that Sync can now be called
	// only once.
	if b.didSync {
		panic("storage.blob.table: Sync called twice for a blob object")
	}

	b.pw.Close()
	b.didSync = true
	return <-b.errCh
}

func (b *tableBlobWriter) Write(p []byte) (n int, err error) {
	return b.pw.Write(p)
}

func (b *tableBlobWriter) Close() error {
	if !b.didSync {
		if err := b.pw.CloseWithError(fmt.Errorf("storage.blob.table: blob closed without Sync")); err != nil {
			panic(err)
		}
	}
	return nil
}

func (s *Store) Create(ctx context.Context, key string, blobSize int64) (module.Blob, error) {
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)

	go func() {
		var pool *sync.Pool
		if blobSize != module.UnknownBlobSize && blobSize <= smallBufferSize {
			pool = &s.smBufPool
		} else {
			pool = &s.lgBufPool
		}

		pb := pool.Get().([]byte)
		defer func() {
			pool.Put(pb)
		}()

		buf := pb[:]
		if len(buf) > int(s.chunkSize) {
			buf = buf[0:s.chunkSize]
		}

		var (
			err    error
			chunks int
			done   bool
			nr     int
		)
		for !done {
			nr, err = io.ReadFull(pr, buf)
			if err != nil {
				done = true
				if err == io.ErrUnexpectedEOF {
					err = nil
				} else {
					err = fmt.Errorf("read: %w", err)
				}
			}
			if err == nil {
				err = s.storage.SetKey(fmt.Sprintf("%s/%d", key, chunks), string(buf[0:nr]))
				chunks++
			}
		}

		if err == nil {
			// value with the unmodified key name contains the total number of chunks
			err = s.storage.SetKey(key, strconv.FormatInt(int64(chunks), 10))
		}

		if err != nil {
			for i := 0; i < chunks; i++ {
				_ = s.storage.RemoveKey(fmt.Sprintf("%s/%d", key, i))
			}

			if err := pr.CloseWithError(err); err != nil {
				panic(err)
			}
		}
		errCh <- err
	}()

	return &tableBlobWriter{
		pw:    pw,
		errCh: errCh,
	}, nil
}

var ErrNotExist = errors.New("requested key does not exist")

type tableBlobReader struct {
	chunks    int
	nextChunk int
	key       string
	buf       string
	storage   module.MutableTable
}

func (b *tableBlobReader) Read(p []byte) (n int, err error) {
	for len(p) > 0 {
		nr := copy(p, b.buf)
		b.buf = b.buf[nr:]
		p = p[nr:]
		n += nr

		if len(b.buf) == 0 {
			if b.nextChunk == b.chunks {
				err = io.EOF
				return
			}

			var exists bool
			b.buf, exists, err = b.storage.Lookup(context.Background(), fmt.Sprintf("%s/%d", b.key, b.nextChunk))
			if err != nil {
				return nr, fmt.Errorf("failed to read chunk: %w", err)
			}
			if !exists {
				return nr, fmt.Errorf("failed to read chunk: %w", ErrNotExist)
			}
			b.nextChunk++
		}
	}

	return
}

func (b *tableBlobReader) Close() error {
	b.buf = ""
	return nil
}

func (s *Store) getChunkCount(ctx context.Context, key string) (int, error) {
	value, ok, err := s.storage.Lookup(ctx, key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, module.ErrNoSuchBlob
	}

	chunks, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid chunk count: %w", err)
	}

	return int(chunks), nil
}

func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	chunks, err := s.getChunkCount(ctx, key)
	if err != nil {
		return nil, err
	}

	return &tableBlobReader{
		storage: s.storage,
		chunks:  chunks,
		key:     key,
	}, nil
}

func (s *Store) delete(ctx context.Context, key string) error {
	chunks, err := s.getChunkCount(ctx, key)
	if err != nil {
		return err
	}

	lastErr := s.storage.RemoveKey(key)
	if lastErr == nil {
		for i := 0; i < chunks; i++ {
			k := fmt.Sprintf("%s/%d", key, i)
			if err := s.storage.RemoveKey(k); err != nil {
				lastErr = err
				s.log.Error("failed to delete key", lastErr, k)
			}
		}
	}

	return lastErr
}

func (s *Store) Delete(ctx context.Context, keys []string) error {
	var lastErr error
	for _, k := range keys {
		if err := s.delete(ctx, k); err != nil {
			lastErr = err
			s.log.Error("failed to delete object", lastErr, k)
		}
	}
	return lastErr
}

func init() {
	var _ module.BlobStore = &Store{}
	module.Register(modName, New)
}
