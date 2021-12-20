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
	"log"
	"net/url"

	"github.com/emersion/go-message/mail"
)

func urlDomains(urls []string) []string {
	for k, u := range urls {
		urlinfo, err := url.Parse(u)
		if err != nil {
			urls[k] = ""
		} else {
			urls[k] = urlinfo.Hostname()
		}
	}
	return urls
}

func extractBodyDomains(r io.Reader) ([]string, error) {
	var domains []string

	// Create a new mail reader
	mr, err := mail.CreateReader(r)
	if err != nil {
		// probably not a MIME message; process as plaintext
		if rs, ok := r.(io.ReadSeeker); ok {
			_, _ = rs.Seek(0, io.SeekStart)
		}
		return extractTextDomains(r)
	}

	// Read each mail's part
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		var ctype string
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			ctype, _, _ = h.ContentType()
			if ctype != "text/html" {
				ctype = "text/plain"
			}
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			log.Printf("Got attachment: %v\n", filename)

			ctype, _, _ = h.ContentType()
		}

		var partDomains []string
		switch ctype {
		case "text/html":
			partDomains, err = extractHTMLDomains(p.Body)
		case "text/plain":
			partDomains, err = extractTextDomains(p.Body)
		default:
			partDomains = partDomains[:0]
		}

		if err != nil {
			return nil, err
		}
		if len(partDomains) > 0 {
			domains = append(domains, partDomains...)
		}
	}

	return domains, nil
}
