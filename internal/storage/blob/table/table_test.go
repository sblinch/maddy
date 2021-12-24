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
	"testing"

	"github.com/foxcpp/maddy/framework/config"
	"github.com/foxcpp/maddy/framework/module"
	"github.com/foxcpp/maddy/internal/storage/blob"
	_ "github.com/foxcpp/maddy/internal/table"
)

func TestTable(t *testing.T) {
	blob.TestStore(t, func() module.BlobStore {
		st := &Store{instName: "test"}
		err := st.Init(config.NewMap(map[string]interface{}{}, config.Node{
			Children: []config.Node{
				{
					Name: "storage",
					Args: []string{"sql_table"},
					Children: []config.Node{
						{
							Name: "driver",
							Args: []string{"sqlite3"},
						},
						{
							Name: "dsn",
							Args: []string{":memory:"},
						},
						{
							Name: "table_name",
							Args: []string{"messages"},
						},
					},
				},
				{
					Name: "chunk_size",
					Args: []string{"1024"},
				},
			},
		}))
		if err != nil {
			panic(err)
		}

		return st
	}, func(store module.BlobStore) {
		s := store.(*Store)
		keys, err := s.storage.Keys()
		if err != nil {
			t.Fatalf("failed to fetch keys: %v", err)
		}
		for _, k := range keys {
			if err := s.storage.RemoveKey(k); err != nil {
				t.Fatalf("failed to remove key: %v", err)
			}
		}
	})

}
