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
	"strconv"
	"strings"

	"github.com/emersion/go-message/textproto"
	"github.com/foxcpp/maddy/framework/buffer"
	"github.com/foxcpp/maddy/framework/config"
	modconfig "github.com/foxcpp/maddy/framework/config/module"
	"github.com/foxcpp/maddy/framework/dns"
	"github.com/foxcpp/maddy/framework/exterrors"
	"github.com/foxcpp/maddy/framework/log"
	"github.com/foxcpp/maddy/framework/module"
	"github.com/foxcpp/maddy/internal/target"
)

const modName = "check.domainbl"

// maximum number of DNS requests in-flight at any given time
const concurrency = 8

type List struct {
	Zone string

	Bits     byte
	ScoreAdj int
}

type Check struct {
	instName  string
	inlineBls []string
	bls       []List

	quarantineThres int
	rejectThres     int

	resolver dns.Resolver
	log      log.Logger
}

func New(modName, instName string, _, inlineArgs []string) (module.Module, error) {
	c := &Check{
		instName:  instName,
		inlineBls: inlineArgs,
		resolver:  dns.DefaultResolver(),
		log:       log.Logger{Name: modName, Debug: log.DefaultLogger.Debug},
	}

	return c, nil
}

func (c *Check) Name() string {
	return modName
}

func (c *Check) InstanceName() string {
	return c.instName
}

func (c *Check) Init(cfg *config.Map) error {
	cfg.Bool("debug", false, false, &c.log.Debug)
	cfg.Int("quarantine_threshold", false, false, 1, &c.quarantineThres)
	cfg.Int("reject_threshold", false, false, 9999, &c.rejectThres)
	cfg.AllowUnknown()
	unknown, err := cfg.Process()
	if err != nil {
		return err
	}

	for _, inlineBl := range c.inlineBls {
		cfg := List{}
		cfg.Zone = inlineBl
		c.bls = append(c.bls, cfg)
	}

	for _, node := range unknown {
		if err := c.readListCfg(node); err != nil {
			return err
		}
	}

	return nil
}

type bitString string

func (b bitString) Bits() (byte, error) {
	if len(b) == 0 {
		return 0, nil
	}
	values := strings.Split(string(b), "+")

	r := int64(0)
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return 0, err
		}
		r += n
	}

	return byte(r), nil
}

func (c *Check) readListCfg(node config.Node) error {
	var (
		listCfg List
		err     error
	)

	cfg := config.NewMap(nil, node)

	var bits bitString
	cfg.String("bits", false, true, "", (*string)(&bits))
	cfg.Int("score", false, false, 1, &listCfg.ScoreAdj)
	if _, err := cfg.Process(); err != nil {
		return err
	}
	listCfg.Bits, err = bits.Bits()
	if err != nil {
		return err
	}

	for _, zone := range append([]string{node.Name}, node.Args...) {
		zoneCfg := listCfg
		zoneCfg.Zone = zone

		c.bls = append(c.bls, zoneCfg)
	}

	return nil
}

type state struct {
	c       *Check
	msgMeta *module.MsgMetadata
	log     log.Logger
}

func (c *Check) CheckStateForMsg(ctx context.Context, msgMeta *module.MsgMetadata) (module.CheckState, error) {
	return &state{
		c:       c,
		msgMeta: msgMeta,
		log:     target.DeliveryLogger(c.log, msgMeta),
	}, nil
}

func (s *state) CheckConnection(ctx context.Context) module.CheckResult {
	return module.CheckResult{}
}

func (s *state) CheckSender(ctx context.Context, addr string) module.CheckResult {
	return module.CheckResult{}
}

func (s *state) CheckRcpt(ctx context.Context, addr string) module.CheckResult {
	return module.CheckResult{}
}

func (s *state) CheckBody(ctx context.Context, hdr textproto.Header, body buffer.Buffer) module.CheckResult {
	bodyR, err := body.Open()
	if err != nil {
		return module.CheckResult{
			Reject: true,
			Reason: exterrors.WithFields(err, map[string]interface{}{"check": modName}),
		}
	}

	domains, err := extractBodyDomains(bodyR)
	if err != nil {
		return module.CheckResult{
			Reason: &exterrors.SMTPError{
				Code:         451,
				EnhancedCode: exterrors.EnhancedCode{4, 7, 0},
				Message:      "Internal error during policy check",
				CheckName:    modName,
				Err:          err,
			},
		}
	}

	score, hits, err := lookupDomainBLs(ctx, s.c.resolver, domains, s.c.bls, concurrency)
	if err != nil {
		return module.CheckResult{
			Reason: &exterrors.SMTPError{
				Code:         451,
				EnhancedCode: exterrors.EnhancedCode{4, 7, 0},
				Message:      "Internal error during policy check",
				CheckName:    modName,
				Err:          err,
			},
		}
	}

	action := modconfig.FailAction{}
	reason := ""
	var misc map[string]interface{}

	if score >= s.c.rejectThres {
		action.Reject = true
		reason = "bl score exceeds reject threshold"
		misc = map[string]interface{}{"bl-score": score, "bl-reject-threshold": s.c.rejectThres, "bl-hits": hits}
	} else if score >= s.c.quarantineThres {
		action.Quarantine = true
		reason = "bl score exceeds quarantine threshold"
		misc = map[string]interface{}{"bl-score": score, "bl-quarantine-threshold": s.c.quarantineThres, "bl-hits": hits}
	} else {
		s.log.DebugMsg("bl results", "bl-score", score, "bl-hits", hits)
		return module.CheckResult{}
	}

	return action.Apply(module.CheckResult{
		Reason: &exterrors.SMTPError{
			Code:         550,
			EnhancedCode: exterrors.EnhancedCode{5, 7, 0},
			Message:      "Message rejected due to local policy",
			CheckName:    modName,
			Reason:       reason,
			Misc:         misc,
		}})
}

func (s *state) Close() error {
	return nil
}

func init() {
	module.Register(modName, New)
}
