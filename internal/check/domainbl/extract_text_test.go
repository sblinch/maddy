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

package domainbl

import (
	"reflect"
	"strings"
	"testing"
)

func Test_extractTextDomains(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		want    []string
		wantErr bool
		bufSize int
	}{
		{"base", "Example.com is at http://example.com.", []string{"example.com"}, false, 40960},
		{"withuri", "Example.com's about page is at http://example.com/about.html.", []string{"example.com"}, false, 40960},
		{"multiple", "Example.com's about page is at http://example.com/about.html and also see http://example.org too.", []string{"example.com", "example.org"}, false, 40960},
		{"none", "Example.com is great", nil, false, 40960},
		{"buffer boundary", `**************************************************************************************************************************************************************************************************************************************************************************** Example.com's about page is at http://example.com/about.html and also see ************************************************************************************************************************************************************************************************************************************************* http://example.org too.`, []string{"example.com", "example.org"}, false, 311},
	}

	var (
		buf         []byte
		lastBufSize int
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if buf == nil || tt.bufSize != lastBufSize {
				buf = make([]byte, tt.bufSize)
				lastBufSize = tt.bufSize
			}
			got, err := extractTextDomains(strings.NewReader(tt.text))
			if (err != nil) != tt.wantErr {
				t.Errorf("extractTextDomains() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractTextDomains() got = %v, want %v", got, tt.want)
			}
		})
	}
}
