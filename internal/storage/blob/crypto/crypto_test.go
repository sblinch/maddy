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
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"

	"github.com/foxcpp/maddy/framework/config"
	"github.com/foxcpp/maddy/framework/module"
	"github.com/foxcpp/maddy/internal/storage/blob"
	_ "github.com/foxcpp/maddy/internal/storage/blob/fs"
	"github.com/foxcpp/maddy/internal/testutils"
)

type cryptoStoreTest struct {
	CryptoStore
	root string
}

func TestCrypto(t *testing.T) {
	blob.TestStore(t, func() module.BlobStore {
		root := testutils.Dir(t)

		st := CryptoStore{instName: "test"}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatalf("Error: %v", err)
		}

		err := st.Init(config.NewMap(map[string]interface{}{}, config.Node{
			Children: []config.Node{
				{
					Name: "msg_store",
					Args: []string{"fs", root},
				},
				{
					Name: "crypto_static_key",
					Args: []string{base64.StdEncoding.EncodeToString(key)},
				},
			},
		}))
		if err != nil {
			panic(err)
		}

		return &cryptoStoreTest{CryptoStore: st, root: root}
	}, func(store module.BlobStore) {
		os.RemoveAll(store.(*cryptoStoreTest).root)
	})

}
