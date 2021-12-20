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
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/foxcpp/maddy/framework/dns"
	"github.com/weppos/publicsuffix-go/publicsuffix"
)

type lookup struct {
	domain string
	bl     List
}
type result struct {
	zone  string
	score int
	err   error
}

var errBadBLResponse = errors.New("bad response from BL")

func lookupDomainBL(ctx context.Context, resolver dns.Resolver, domain string, bl List) (int, error) {
	var blDomain string

	blDomain = domain + "." + bl.Zone

	addrs, err := resolver.LookupHost(ctx, blDomain)
	if err != nil {
		if e, ok := err.(*net.DNSError); ok && e.IsNotFound {
			return 0, nil
		}
		return 0, err
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return 0, errBadBLResponse
		}

		res := ip.To4()[3]

		for bit := 0; bit < 8; bit++ {
			n := byte(1 << bit)
			if (bl.Bits&n != 0) && (res&n != 0) {
				return bl.ScoreAdj, nil
			}
		}
	}

	return 0, nil

}

func cleanupDomains(domains []string) []string {
	unique := make(map[string]struct{})
	for _, domain := range domains {
		if domain == "" {
			continue
		}

		ip := net.ParseIP(domain)
		if ip != nil {
			v4 := ip.To4()
			if v4 == nil {
				// current domain BLs only support IPv4 addresses
				continue
			}

			// domain BLs support bare IP addresses in reverse octet order
			v4[3], v4[2], v4[1], v4[0] = v4[0], v4[1], v4[2], v4[3]
			domain = v4.String()
		} else {
			base, err := publicsuffix.Domain(domain)
			if err == nil {
				domain = base
			}
		}
		unique[domain] = struct{}{}
	}
	domains = domains[:0]
	for domain, _ := range unique {
		domains = append(domains, domain)
	}

	return domains
}

func lookupDomainBLs(ctx context.Context, resolver dns.Resolver, domains []string, bls []List, concurrency int) (int, []string, error) {
	domains = cleanupDomains(domains)
	lookups := len(domains) * len(bls)
	if concurrency > lookups {
		concurrency = lookups
	}

	resultC := make(chan result)

	var (
		score int
		hits  []string
		err   error
		mu    sync.Mutex
	)

	doneScores := make(chan struct{})

	go func() {
		defer func() {
			if rcvr := recover(); rcvr != nil {
				mu.Lock()
				err = fmt.Errorf("%v", rcvr)
				mu.Unlock()
			}
			close(doneScores)
		}()

		for res := range resultC {
			if res.err != nil {
				mu.Lock()
				err = fmt.Errorf("%s: %v", res.zone, res.err)
				mu.Unlock()
			}
			if res.score != 0 {
				score += res.score
				hits = append(hits, res.zone)
			}
		}
	}()

	lookupC := make(chan lookup)

	wg := sync.WaitGroup{}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer func() {
				if rcvr := recover(); rcvr != nil {
					mu.Lock()
					err = fmt.Errorf("%v", rcvr)
					mu.Unlock()
				}
				wg.Done()
			}()

			select {
			case <-ctx.Done():
				mu.Lock()
				err = context.Canceled
				mu.Unlock()
				return
			case job := <-lookupC:
				score, err := lookupDomainBL(ctx, resolver, job.domain, job.bl)
				resultC <- result{zone: job.bl.Zone, score: score, err: err}
			}
		}()
	}

	for _, domain := range domains {
		for _, bl := range bls {
			lookupC <- lookup{domain, bl}
		}
	}
	close(lookupC)

	wg.Wait()
	close(resultC)

	<-doneScores

	return score, hits, err
}
