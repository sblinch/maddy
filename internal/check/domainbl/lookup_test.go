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
	"context"
	"net"
	"testing"
)

type mockBLResolver struct {
	addr string
	err  error
}

func (m *mockBLResolver) LookupHost(ctx context.Context, host string) (addrs []string, err error) {
	if m.err != nil {
		return []string{}, m.err
	}
	return []string{m.addr}, nil
}
func (m *mockBLResolver) LookupAddr(ctx context.Context, addr string) (names []string, err error) {
	return []string{}, nil
}
func (m *mockBLResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	return []*net.MX{}, nil
}
func (m *mockBLResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return []string{}, nil
}
func (m *mockBLResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return []net.IPAddr{}, nil
}

func Test_lookupDomainBL(t *testing.T) {
	tests := []struct {
		name         string
		mockResolver mockBLResolver
		domain       string
		wantScore    int
		wantErr      bool
	}{
		{"bl-hit-matching-bits", mockBLResolver{"127.0.0.254", nil}, "turrible-spammer.example.org", 1, false},
		{"bl-hit-mismatched-bits", mockBLResolver{"127.0.0.64", nil}, "turrible-spammer.example.org", 0, false},
		{"bl-miss", mockBLResolver{"", &net.DNSError{IsNotFound: true}}, "goodguy.example.org", 0, false},
		{"bl-error", mockBLResolver{"", &net.DNSError{IsTemporary: true}}, "failure.example.org", 0, true},
	}

	ctx := context.Background()
	bl := List{Zone: "domainbl.example.org", Bits: 128, ScoreAdj: 1}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, err := lookupDomainBL(ctx, &tt.mockResolver, tt.domain, bl)
			if (err != nil) != tt.wantErr {
				t.Errorf("lookupDomainBL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotScore != tt.wantScore {
				t.Errorf("lookupDomainBL() got = %v, want %v", gotScore, tt.wantScore)
			}
		})
	}
}
