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

package pattern

import (
	"regexp"
	"testing"
)

func Test_valueMatchesPattern(t *testing.T) {
	reCache := make(map[string]*regexp.Regexp)
	type args struct {
		value   string
		pattern string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{"regexp", args{"this is a test", "/test$/"}, true, false},
		{"regexp-flag", args{"this is a TEST", "/test$/i"}, true, false},
		{"keyword", args{"this is a test", "*test*"}, true, false},
		{"exact", args{"this is a test", "this is a test"}, true, false},
		{"cidr", args{"10.0.0.1", "cidr:10.0.0.0/8"}, true, false},
		{"neg-regexp", args{"this is a test", "/foo/"}, false, false},
		{"neg-keyword", args{"this is a test", "*foo*"}, false, false},
		{"neg-exact", args{"this is a test", "this is not a test"}, false, false},
		{"neg-cidr", args{"11.0.0.1", "cidr:10.0.0.0/8"}, false, false},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := valueMatchesPattern(reCache, tt.args.value, tt.args.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("valueMatchesPattern() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("valueMatchesPattern() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_splitLast(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		want  string
		want1 string
	}{
		{"two words", "alpha bravo", "alpha", "bravo"},
		{"three words", "alpha bravo gamma", "alpha bravo", "gamma"},
		{"quoted and action", "\"alpha bravo\" gamma", "alpha bravo", "gamma"},
		{"one word", "alpha", "alpha", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := splitLast(tt.s)
			if got != tt.want {
				t.Errorf("splitLast() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("splitLast() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
