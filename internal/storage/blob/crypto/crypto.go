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

package crypto

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"runtime"

	"github.com/foxcpp/maddy/framework/config"
	modconfig "github.com/foxcpp/maddy/framework/config/module"
	"github.com/foxcpp/maddy/framework/log"
	"github.com/foxcpp/maddy/framework/module"
	"github.com/minio/sio"
	"golang.org/x/crypto/argon2"
)

const modName = "storage.blob.crypto"

// CryptoStore wraps another BlobStore to transparently add encryption.
type CryptoStore struct {
	instName string
	log      log.Logger

	storage module.BlobStore

	cryptoPassphrase string
	cryptoStaticKey  []byte
	cryptoTime       uint32
	cryptoMemory     uint32
	cryptoThreads    uint8
}

func New(_, instName string, _, inlineArgs []string) (module.Module, error) {
	if len(inlineArgs) != 0 {
		return nil, fmt.Errorf("%s: expected 0 arguments", modName)
	}
	return &CryptoStore{
		instName: instName,
		log:      log.Logger{Name: modName, Debug: log.DefaultLogger.Debug},
	}, nil
}

func (s CryptoStore) Name() string {
	return modName
}

func (s CryptoStore) InstanceName() string {
	return s.instName
}

var (
	errStaticKeyLen = errors.New("base64-decoded static key must be exactly 32 bytes in length")
	errBothKeyTypes = errors.New("cannot specify both passphrase and static key")
)

func (s *CryptoStore) Init(cfg *config.Map) error {
	var (
		cryptoStaticKey string
	)
	cfg.Custom("msg_store", false, false, func() (interface{}, error) {
		var store module.BlobStore
		err := modconfig.ModuleFromNode("storage.blob", []string{"fs", "messages"},
			config.Node{}, nil, &store)
		return store, err
	}, func(m *config.Map, node config.Node) (interface{}, error) {
		var store module.BlobStore
		err := modconfig.ModuleFromNode("storage.blob", node.Args,
			node, m.Globals, &store)
		return store, err
	}, &s.storage)

	cfg.String("crypto_static_key", false, false, "", &cryptoStaticKey)

	// In practice the following options will probably never be used. To derive a secure key, the passphrase needs to be
	// run through a KDF with a unique salt for every encrypt/decrypt operation, and the KDF is (by design) very slow --
	// it requires at least one full second per invocation. This makes message storage/retrieval impractically slow.
	// Might just want to remove this entirely.
	cfg.String("crypto_passphrase", false, false, "", &s.cryptoPassphrase)
	cfg.UInt32("crypto_time", false, false, 1, &s.cryptoTime)
	cfg.UInt32("crypto_memory", false, false, 64, &s.cryptoMemory)

	if _, err := cfg.Process(); err != nil {
		return err
	}

	if cryptoStaticKey != "" {
		if s.cryptoPassphrase != "" {
			return errBothKeyTypes
		}
		var err error
		s.cryptoStaticKey, err = base64.StdEncoding.DecodeString(cryptoStaticKey)
		if err != nil {
			return err
		}
		if len(s.cryptoStaticKey) != 32 {
			return errStaticKeyLen
		}
		s.log.DebugMsg("using static key for table storage crypto")
	} else if s.cryptoPassphrase != "" {
		s.log.DebugMsg("using passphrase for table storage crypto")
	} else {
		s.log.DebugMsg("using no crypto for table storage")
	}
	cpus := runtime.NumCPU()
	if cpus > 255 {
		cpus = 255
	}
	s.cryptoThreads = uint8(cpus)

	return nil
}

func (s *CryptoStore) cryptoKey(key string) []byte {
	if s.cryptoStaticKey != nil {
		return s.cryptoStaticKey
	} else if s.cryptoPassphrase != "" {
		salt := []byte(key)
		return argon2.IDKey([]byte(s.cryptoPassphrase), salt, s.cryptoTime, s.cryptoMemory*1024, s.cryptoThreads, 32)
	} else {
		return nil
	}
}

func (s *CryptoStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	r, err := s.storage.Open(ctx, key)
	if err != nil {
		return nil, err
	}

	cryptoKey := s.cryptoKey(key)
	if cryptoKey == nil {
		return r, nil
	}
	decrypted, err := sio.DecryptReader(r, sio.Config{
		Key: cryptoKey,
	})
	if err != nil {
		return nil, err
	}

	return struct {
		io.Reader
		io.Closer
	}{Reader: decrypted, Closer: r}, nil
}

type cryptoBlob struct {
	b       module.Blob
	w       io.WriteCloser
	didSync bool
}

func (b *cryptoBlob) Sync() error {
	// We do this in Sync instead of Close because
	// backend may not actually check the error of Close.
	// The problematic restriction is that Sync can now be called
	// only once.
	if b.didSync {
		panic("storage.blob.crypto: Sync called twice for a blob object")
	}

	b.didSync = true

	return b.w.Close()
}

func (b *cryptoBlob) Write(p []byte) (n int, err error) {
	return b.w.Write(p)
}

func (b *cryptoBlob) Close() error {
	if !b.didSync {
		return fmt.Errorf("storage.blob.crypto: blob closed without Sync")
	}

	return nil
}

func (s *CryptoStore) Create(ctx context.Context, key string, blobSize int64) (module.Blob, error) {

	if blobSize != module.UnknownBlobSize {
		encSize, err := sio.EncryptedSize(uint64(blobSize))
		if err != nil {
			return nil, err
		}
		blobSize = int64(encSize)
	}

	b, err := s.storage.Create(ctx, key, blobSize)
	if err != nil {
		return nil, err
	}

	cryptoKey := s.cryptoKey(key)
	if cryptoKey == nil {
		return b, nil
	}

	w, err := sio.EncryptWriter(b, sio.Config{
		Key: cryptoKey,
	})

	return &cryptoBlob{
		b: b,
		w: w,
	}, nil
}

func (s *CryptoStore) Delete(ctx context.Context, keys []string) error {
	return s.storage.Delete(ctx, keys)
}

func init() {
	var _ module.BlobStore = &CryptoStore{}
	module.Register(modName, New)
}
