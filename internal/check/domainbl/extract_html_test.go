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

func Test_extractHTMLDomains(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		want    []string
		wantErr bool
	}{
		{"a-href", "Check out <a href=\"http://example.com\">example.com</a>.", []string{"example.com"}, false},
		{"script-src", "<script src=\"http://example.com\"></script>", []string{"example.com"}, false},
		{"a-img", "<a href=\"http://www.example.org\"><img src=\"http://www.example.com/foo.jpg\"></a>.", []string{"www.example.org", "www.example.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractHTMLDomains(strings.NewReader(tt.html))
			if (err != nil) != tt.wantErr {
				t.Errorf("extractHTMLDomains() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractHTMLDomains() got = %v, want %v", got, tt.want)
			}
		})
	}
}
