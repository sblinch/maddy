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
	"strings"

	"github.com/foxcpp/maddy/framework/module"
)

func checkHostAddress(ctx context.Context, hostTable module.Table, address string) (matchResult, error) {
	if len(address) == 0 {
		return matchResult{}, nil
	}

	sep := byte('.')

	completeAddress := address
	for {
		action, exists, err := hostTable.Lookup(ctx, address)
		if err != nil {
			return matchResult{}, err
		}
		if exists {
			return matchResult{Matches: true, Pattern: "", Value: completeAddress, Action: action}, nil
		}

		p := strings.LastIndexByte(address[0:len(address)-1], sep)
		if p == -1 {
			sep = ':'
			p = strings.LastIndexByte(address[0:len(address)-1], sep)
			if p == -1 {
				return matchResult{}, nil
			}
		}
		address = address[0 : p+1]
	}
}

func checkHostTable(ctx context.Context, hostTable module.Table, name, hostAddress string) (matchResult, error) {
	result, err := checkHostAddress(ctx, hostTable, hostAddress)
	if err != nil {
		return matchResult{}, err
	} else if result.Matches {
		result.Pattern = name
		return result, err
	} else {
		return matchResult{}, nil
	}
}
