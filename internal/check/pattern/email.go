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
	"net/mail"

	"github.com/emersion/go-message/textproto"
	"github.com/foxcpp/maddy/framework/address"
	"github.com/foxcpp/maddy/framework/module"
)

func (c *Check) checkEmailAddress(ctx context.Context, emailTable module.Table, email string) (matchResult, error) {
	if len(email) == 0 {
		return matchResult{}, nil
	}

	action, exists, err := emailTable.Lookup(ctx, email)
	if err != nil {
		return matchResult{}, err
	}
	if exists {
		return matchResult{Matches: true, Pattern: "", Value: email, Action: action}, nil
	}

	user, domain, err := address.Split(email)
	if err != nil {
		return matchResult{}, err
	}

	action, exists, err = emailTable.Lookup(ctx, user+"@")
	if err != nil {
		return matchResult{}, err
	}
	if exists {
		return matchResult{Matches: true, Pattern: "", Value: email, Action: action}, nil
	}

	action, exists, err = emailTable.Lookup(ctx, "@"+domain)
	if err != nil {
		return matchResult{}, err
	}
	if exists {
		return matchResult{Matches: true, Pattern: "", Value: email, Action: action}, nil
	}

	return matchResult{}, nil
}

func (c *Check) checkEmailTable(ctx context.Context, emailTable module.Table, key, emailAddress string, normFunc func(string) (string, error)) (matchResult, error) {
	normEmailAddress, err := normFunc(emailAddress)
	if err != nil {
		return matchResult{}, err
	}

	result, err := c.checkEmailAddress(ctx, emailTable, normEmailAddress)
	if err != nil {
		return matchResult{}, err
	} else if result.Matches {
		result.Pattern = key
		return result, nil
	} else {
		return matchResult{}, nil
	}
}

func getEmailAddresses(headerNames []string, hdr textproto.Header) (map[string]string, error) {
	values := make(map[string]string)
	for _, name := range headerNames {
		if !hdr.Has(name) {
			continue
		}
		headerValue := hdr.Get(name)
		if len(headerValue) == 0 {
			continue
		}

		list, err := mail.ParseAddressList(headerValue)
		if err != nil {
			return nil, err
		}

		for _, item := range list {
			if _, exists := values[item.Address]; !exists {
				values[item.Address] = name
			}
		}
	}

	return values, nil
}
