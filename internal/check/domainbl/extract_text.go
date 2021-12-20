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
	"io"

	"mvdan.cc/xurls/v2"
)

const (
	maxDomainLength            = 253
	maxExpectedURLSchemeLength = 32
)

func extractTextDomainsBuf(r io.Reader, buf []byte) ([]string, error) {
	var domains []string

	xu := xurls.Strict()

	hold := 0

	for {
		nr, err := r.Read(buf[hold:])

		if nr > 0 {
			data := buf[0 : hold+nr]
			matches := xu.FindAllString(string(data), -1)
			domains = append(domains, matches...)

			hold = maxDomainLength + maxExpectedURLSchemeLength
			if hold > len(data) {
				hold = len(data)
			}
			hold = copy(buf, data[len(data)-hold:])
		}

		if err != nil {
			if err == io.EOF {
				return urlDomains(domains), nil
			}
			return nil, err
		}
	}
}

func extractTextDomains(r io.Reader) ([]string, error) {
	buf := make([]byte, 40960)
	return extractTextDomainsBuf(r, buf)
}
