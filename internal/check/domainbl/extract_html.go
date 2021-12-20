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
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// parse <img srcset="url1 resolution1, url2 resolution2">
func parseSrcSet(srcset string) []string {
	var urls []string
	items := strings.Split(srcset, ",")
	if len(items) == 1 && items[0] == "" {
		return []string{}
	}

	for _, item := range items {
		parts := strings.SplitN(strings.TrimSpace(item), " ", 2)
		url := parts[0]
		if len(url) > 0 {
			urls = append(urls, parts[0])
		}
	}
	return urls
}

// parse <object archive=url> or <object archive="url1 url2 url3">
func parseArchive(archive string) []string {
	urls := strings.Split(strings.TrimSpace(archive), " ")
	if len(urls) == 1 && urls[0] == "" {
		return []string{}
	} else {
		for k, u := range urls {
			urls[k] = strings.TrimSpace(u)
		}
		return urls
	}
}

// parse <meta http-equiv="refresh" content="seconds; url">
func parseMetaContent(content string) string {
	parts := strings.Split(content, ";")
	if len(parts) < 2 {
		return ""
	}
	u := strings.TrimSpace(parts[1])
	return u
}

// parse <div style="background: url(image.png)"> (poorly)
var reURLStyle = regexp.MustCompile("url\\((.*?)\\)")

func parseInlineStyle(style string) []string {
	var urls []string
	matches := reURLStyle.FindAllStringSubmatch(style, -1)
	for _, match := range matches {
		urls = append(urls, strings.TrimSpace(match[1]))
	}
	return urls
}

func extractHTMLDomains(r io.Reader) ([]string, error) {
	var urls []string
	ht := html.NewTokenizer(r)
	for {
		tokenType := ht.Next()
		switch tokenType {
		case html.ErrorToken:
			return urlDomains(urls), nil
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := ht.Token()

			for _, a := range tok.Attr {
				switch a.Key {
				case "href", "src", "codebase", "cite", "background", "action", "longdesc", "profile", "usemap", "classid", "data", "formaction", "icon", "manifest", "poster":
					urls = append(urls, a.Val)

				case "srcset":
					urls = append(urls, parseSrcSet(a.Val)...)

				case "archive":
					urls = append(urls, parseArchive(a.Val)...)

				case "content":
					if tok.Data == "meta" {
						if url := parseMetaContent(a.Val); url != "" {
							urls = append(urls, url)
						}
					}
				case "style":
					urls = append(urls, parseInlineStyle(a.Val)...)
				}
			}
		}
	}
}
