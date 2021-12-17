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
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/foxcpp/maddy/framework/module"
)

// splitLast locates the last space character in s and splits s at that point, returning the pieces before and after it
func splitLast(s string) (string, string) {
	s = strings.TrimSpace(s)
	p := strings.LastIndexByte(s, ' ')
	if p == -1 {
		return s, ""
	}
	first := strings.TrimSpace(s[0:p])
	if len(first) > 1 && first[0] == '"' {
		var err error
		first, err = strconv.Unquote(first)
		if err != nil {
			return "", ""
		}
	}

	return first, s[p+1:]
}

// checkPatternTable checks matchTable for a regular expression matching key, normalizes value with normFunc, compares
// the normalized value to the regular expression(s) (caching the compiled regexp in reCache), and returns either a
// match or an error
func checkPatternTable(ctx context.Context, matchTable module.MultiTable, reCache map[string]*regexp.Regexp, key, value string, normFunc func(string) (string, error)) (matchResult, error) {
	normValue, err := normFunc(value)
	if err != nil {
		return matchResult{}, err
	}

	rules, err := matchTable.LookupMulti(ctx, key)
	if err != nil {
		return matchResult{}, err
	}

	for _, rule := range rules {
		pattern, action := splitLast(rule)

		matches, err := valueMatchesPattern(reCache, normValue, pattern)
		if err != nil {
			return matchResult{}, err
		} else if matches {
			return matchResult{Matches: true, Type: key, Pattern: pattern, Value: normValue, Action: action}, nil
		}
	}

	return matchResult{}, nil
}

// convertToGoRegexp converts a regular expression formatted as /pattern/flags to (?flags)pattern; for example,
// `/foo/` or `/foo/i` to `foo` or `(?i)foo`, respectively
func convertToGoRegexp(pattern string) string {
	pattern = pattern[1:]
	n := len(pattern) - 1
	for n >= 0 && pattern[n] != '/' && pattern[n] >= 'a' && pattern[n] <= 'z' {
		n--
	}
	if n < 0 {
		return ""
	}
	if n < len(pattern)-1 {
		b := strings.Builder{}
		for i := n + 1; i < len(pattern); i++ {
			switch pattern[i] {
			case 'i', 'm', 's', 'U':
				b.WriteString("(?")
				b.WriteByte(pattern[i])
				b.WriteByte(')')
			}
		}
		b.WriteString(pattern[0:n])
		return b.String()
	} else {
		return pattern[0:n]
	}
}

var ErrBadPattern = errors.New("invalid pattern")

// valueMatchesPattern compares value to the regular expression pattern, caching the compiled regular expression in
// reCache; returns true or false to indicate whether it matched, or an error on error
func valueMatchesPattern(reCache map[string]*regexp.Regexp, value, pattern string) (bool, error) {
	if len(pattern) == 0 {
		return false, fmt.Errorf("%v: pattern is empty", ErrBadPattern)
	}

	// regular expression pattern /pattern/ eg: /^[a-zA-Z0-9]+$/
	if len(pattern) > 1 && pattern[0] == '/' {
		re := reCache[pattern]
		if re == nil {
			if patternRegexp := convertToGoRegexp(pattern); patternRegexp != "" {
				var err error
				re, err = regexp.Compile(patternRegexp)
				if err != nil {
					return false, fmt.Errorf("regexp pattern %q: %v", pattern, err)
				}
				reCache[pattern] = re
			}
		}

		if re != nil {
			return re.MatchString(value), nil
		}
	}

	// substring pattern *keyword*, match anywhere in string
	if len(pattern) > 1 && pattern[0] == '*' && pattern[len(pattern)-1] == '*' {
		pattern = pattern[1 : len(pattern)-1]
		return strings.Contains(value, pattern), nil
	}

	// suffix pattern *keyword
	if len(pattern) > 1 && pattern[0] == '*' {
		pattern = pattern[1:]
		return strings.HasSuffix(value, pattern), nil
	}

	// prefix pattern keyword*
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		pattern = pattern[0 : len(pattern)-1]
		return strings.HasPrefix(value, pattern), nil
	}

	// CIDR pattern cidr:CIDRMASK eg: cidr:10.10.0.0/16
	if strings.HasPrefix(pattern, "cidr:") {
		pattern := strings.TrimPrefix(pattern, "cidr:")
		_, cidrNet, err := net.ParseCIDR(pattern)
		if err != nil {
			return false, fmt.Errorf("CIDR pattern %q: %v", pattern, err)
		}
		valueIP := net.ParseIP(value)
		return cidrNet.Contains(valueIP), nil
	}

	// exact match
	return value == pattern, nil
}
