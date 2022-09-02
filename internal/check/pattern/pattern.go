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

	"github.com/emersion/go-message/textproto"
	"github.com/emersion/go-smtp"
	"github.com/foxcpp/maddy/framework/buffer"
	"github.com/foxcpp/maddy/framework/config"
	modconfig "github.com/foxcpp/maddy/framework/config/module"
	"github.com/foxcpp/maddy/framework/exterrors"
	"github.com/foxcpp/maddy/framework/log"
	"github.com/foxcpp/maddy/framework/module"
	"github.com/foxcpp/maddy/internal/authz"
	"github.com/foxcpp/maddy/internal/table"
	"github.com/foxcpp/maddy/internal/target"
)

const modName = "check.pattern"

type Check struct {
	instName string
	log      log.Logger

	matchSender    module.Table
	matchRecipient module.Table
	matchHost      module.Table
	match          module.Table

	checkEarly bool

	emailNorm  func(string) (string, error)
	headerNorm func(string) (string, error)

	errAction modconfig.FailAction

	reCache map[string]*regexp.Regexp
}

func New(_, instName string, _, inlineArgs []string) (module.Module, error) {
	return &Check{
		instName: instName,
		log:      log.Logger{Name: modName, Debug: log.DefaultLogger.Debug},
	}, nil
}

func (c *Check) Name() string {
	return modName
}

func (c *Check) InstanceName() string {
	return c.instName
}

func (c *Check) Init(cfg *config.Map) error {
	cfg.Bool("debug", true, false, &c.log.Debug)

	cfg.Bool("check_early", true, true, &c.checkEarly)

	cfg.Custom("match_sender", false, false, func() (interface{}, error) {
		return table.NewStatic("", "", nil, nil)
	}, modconfig.TableDirective, &c.matchSender)

	cfg.Custom("match_recipient", false, false, func() (interface{}, error) {
		return table.NewStatic("", "", nil, nil)
	}, modconfig.TableDirective, &c.matchRecipient)

	cfg.Custom("match_host", false, false, func() (interface{}, error) {
		return table.NewStatic("", "", nil, nil)
	}, modconfig.TableDirective, &c.matchHost)

	cfg.Custom("match", false, false, func() (interface{}, error) {
		return table.NewStatic("", "", nil, nil)
	}, modconfig.TableDirective, &c.match)

	cfg.Custom("error_action", false, false,
		func() (interface{}, error) {
			return modconfig.FailAction{}, nil
		}, modconfig.FailActionDirective, &c.errAction)

	var (
		emailNormalize  string
		headerNormalize string
		ok              bool
	)
	cfg.String("email_normalize", false, false, "precis_casefold_email", &emailNormalize)
	cfg.String("header_normalize", false, false, "noop", &headerNormalize)

	if _, err := cfg.Process(); err != nil {
		return err
	}

	c.emailNorm, ok = authz.NormalizeFuncs[emailNormalize]
	if !ok {
		return fmt.Errorf("%v: unknown normalization function: %v", modName, emailNormalize)
	}

	c.headerNorm, ok = authz.NormalizeFuncs[headerNormalize]
	if !ok {
		return fmt.Errorf("%v: unknown normalization function: %v", modName, headerNormalize)
	}

	return nil
}

type state struct {
	c       *Check
	msgMeta *module.MsgMetadata
	log     log.Logger
}

func (c *Check) CheckStateForMsg(_ context.Context, msgMeta *module.MsgMetadata) (module.CheckState, error) {
	return &state{
		c:       c,
		msgMeta: msgMeta,
		log:     target.DeliveryLogger(c.log, msgMeta),
	}, nil
}

var ErrInvalidAction = errors.New("invalid action")

func (s *state) matchCheckResult(r matchResult) module.CheckResult {
	cr := module.CheckResult{}

	switch r.Action {
	case "reject":
		cr.Reject = true
		cr.Reason = &exterrors.SMTPError{
			Code:         550,
			EnhancedCode: exterrors.EnhancedCode{5, 7, 1},
			Message:      "Message rejected due to local policy",
			CheckName:    modName,
			Misc:         map[string]interface{}{"pattern-type": r.Type, "pattern-matched": r.Pattern, "pattern-value": r.Value},
		}
	case "quarantine":
		cr.Quarantine = true
		cr.Reason = &exterrors.SMTPError{
			CheckName: modName,
			Misc:      map[string]interface{}{"pattern-type": r.Type, "pattern-matched": r.Pattern, "pattern-value": r.Value},
		}
	case "ignore", "safelist":
		// ignore
	default:
		cr = s.errorCheckResult(ErrInvalidAction, map[string]interface{}{"action": r.Action})
	}
	return cr
}

func (s *state) errorCheckResult(err error, misc map[string]interface{}) module.CheckResult {
	return s.c.errAction.Apply(module.CheckResult{
		Reason: &exterrors.SMTPError{
			Code:         454,
			EnhancedCode: exterrors.EnhancedCode{4, 7, 1},
			Message:      "Internal error during policy check",
			CheckName:    modName,
			Err:          err,
			Misc:         misc,
		}})
}

func (c *Check) checkAddress(ctx context.Context, addr string) (matchResult, error) {
	key := "remote-addr"
	c.log.DebugMsg("checking host", "host", addr, "in", ctx.Value(entrypointKey{}))
	result, err := checkHostTable(ctx, c.matchHost, key, addr)
	if err != nil {
		return matchResult{}, err
	}
	if result.Matches {
		result.Type = "host"
		return result, nil
	}

	return matchResult{}, nil
}

func (c *Check) checkMsgMeta(ctx context.Context, msgMeta *module.MsgMetadata) (matchResult, error) {
	remoteAddr, _, err := net.SplitHostPort(msgMeta.Conn.RemoteAddr.String())
	if err != nil {
		remoteAddr = ""
	}

	noop := func(s string) (string, error) {
		return s, nil
	}

	m, ok := c.match.(module.MultiTable)
	if ok {
		key := "helo-hostname"
		result, err := checkPatternTable(ctx, m, c.reCache, key, msgMeta.Conn.Hostname, noop)
		if err == nil && !result.Matches {
			key = "remote-addr"
			result, err = checkPatternTable(ctx, m, c.reCache, key, remoteAddr, noop)
		}
		if err == nil && !result.Matches {
			key = "auth-user"
			result, err = checkPatternTable(ctx, m, c.reCache, key, msgMeta.Conn.AuthUser, noop)
		}
		if err == nil && !result.Matches {
			key = "proto"
			result, err = checkPatternTable(ctx, m, c.reCache, key, msgMeta.Conn.Proto, noop)
		}
		if err == nil && !result.Matches {
			rdnsNameI, rdnsErr := msgMeta.Conn.RDNSName.GetContext(ctx)
			if rdnsErr == nil {
				if rdnsName, ok := rdnsNameI.(string); ok {
					key = "rdnsname"
					result, err = checkPatternTable(ctx, m, c.reCache, key, rdnsName, noop)
				}
			}
		}

		if err != nil {
			return matchResult{}, err
		}
		if result.Matches {
			result.Type = "pattern_" + result.Type
			return result, nil
		}
	}

	return c.checkAddress(ctx, remoteAddr)
}

func (c *Check) CheckSafelist(ctx context.Context, msgMeta *module.MsgMetadata) module.SafelistCheckResult {
	ctx = context.WithValue(ctx, entrypointKey{}, "check-safelist")
	result, _ := c.checkMsgMeta(ctx, msgMeta)

	if !(result.Matches && result.Action == "safelist") {
		result, _ = c.checkEmailTable(ctx, c.matchSender, "mail-from", msgMeta.OriginalFrom, c.emailNorm)
		if result.Matches {
			result.Type = "sender"
		}
	}
	if !(result.Matches && result.Action == "safelist") {
		for _, recipient := range msgMeta.OriginalRcpts {
			result, _ = c.checkEmailTable(ctx, c.matchRecipient, "rcpt-to", recipient, c.emailNorm)
			if result.Matches {
				result.Type = "sender"
			}
		}
	}

	if result.Matches && result.Action == "safelist" {
		c.log.DebugMsg("message matches safelisted pattern", "type", result.Type, "pattern", result.Pattern, "value", result.Value, "in", ctx.Value(entrypointKey{}))
		h := textproto.Header{}
		h.Set("X-Safelist-Pattern", result.Type+" "+result.Value)
		return module.SafelistCheckResult{
			Safelist: true,
			Header:   h,
		}
	}

	return module.SafelistCheckResult{}
}

type entrypointKey struct{}

// CheckConnection implements module.EarlyCheck, and allows rejecting connections from a host with a given IP address
// before the SMTP session even begins.
func (c *Check) CheckConnection(ctx context.Context, state *smtp.ConnectionState) error {
	ctx = context.WithValue(ctx, entrypointKey{}, "check-connection")
	remoteAddrPort := state.RemoteAddr.String()
	remoteAddr, _, err := net.SplitHostPort(remoteAddrPort)
	if err != nil {
		return err
	}
	result, err := c.checkAddress(ctx, remoteAddr)
	if err != nil {
		c.log.DebugMsg("error checking host address", "host", remoteAddr, "error", err, "in", ctx.Value(entrypointKey{}))
		if !c.errAction.Reject {
			err = nil
		}
		return err
	}
	if result.Matches && result.Action == "reject" {
		c.log.DebugMsg("remote address matched reject pattern; rejecting", "pattern-type", "host", "pattern-matched", result.Pattern, "pattern-value", result.Value, "in", ctx.Value(entrypointKey{}))
		return fmt.Errorf("host %s matches pattern %s", remoteAddr, result.Pattern)
	}

	return nil
}

// CheckConnection checks the msgMeta properties of a message against the pattern tables.
func (s *state) CheckConnection(ctx context.Context) module.CheckResult {
	ctx = context.WithValue(ctx, entrypointKey{}, "state.check-connection")
	result, err := s.c.checkMsgMeta(ctx, s.msgMeta)
	if err != nil {
		return s.errorCheckResult(err, map[string]interface{}{"match": "host", "key": result.Pattern})
	}
	if result.Matches {
		result.Type = "host"
		return s.matchCheckResult(result)
	}

	return module.CheckResult{}
}

// CheckSender checks the MAIL FROM: sender of the message against the sender pattern table.
func (s *state) CheckSender(ctx context.Context, fromEmail string) module.CheckResult {
	ctx = context.WithValue(ctx, entrypointKey{}, "state.check-sender")
	if s.msgMeta.Conn == nil {
		s.log.Msg("skipping locally generated message")
		return module.CheckResult{}
	}

	key := "mail-from"
	result, err := s.c.checkEmailTable(ctx, s.c.matchSender, key, fromEmail, s.c.emailNorm)
	if err != nil {
		return s.errorCheckResult(err, map[string]interface{}{"match": "sender", "key": key})
	}
	if result.Matches {
		result.Type = "sender"
		return s.matchCheckResult(result)
	}

	return s.checkSenderAddressPattern(ctx, fromEmail)
}

func (s *state) checkSenderAddressPattern(ctx context.Context, emailAddress string) module.CheckResult {
	matchTable, haveMatchMultiTable := s.c.match.(module.MultiTable)
	if haveMatchMultiTable {
		key := "sender-address"
		result, err := checkPatternTable(ctx, matchTable, s.c.reCache, key, emailAddress, s.c.emailNorm)
		if err != nil {
			return s.errorCheckResult(err, map[string]interface{}{"match": "pattern", "key": key})
		}
		if result.Matches {
			result.Type = "pattern_" + result.Type
			return s.matchCheckResult(result)
		}
	}
	return module.CheckResult{}
}

// CheckRcpt checks the RCPT TO: recipient of the message against the recipient pattern table.
func (s *state) CheckRcpt(ctx context.Context, toEmail string) module.CheckResult {
	ctx = context.WithValue(ctx, entrypointKey{}, "state.check-rcpt")
	key := "rcpt-to"
	result, err := s.c.checkEmailTable(ctx, s.c.matchRecipient, key, toEmail, s.c.emailNorm)
	if err != nil {
		return s.errorCheckResult(err, map[string]interface{}{"match": "recipient", "key": key})
	}
	if result.Matches {
		result.Type = "recipient"
		return s.matchCheckResult(result)
	}

	return s.checkRecipientAddressPattern(ctx, toEmail)
}

func (s *state) checkRecipientAddressPattern(ctx context.Context, emailAddress string) module.CheckResult {
	matchTable, haveMatchMultiTable := s.c.match.(module.MultiTable)
	if haveMatchMultiTable {
		key := "recipient-address"
		result, err := checkPatternTable(ctx, matchTable, s.c.reCache, key, emailAddress, s.c.emailNorm)
		if err != nil {
			return s.errorCheckResult(err, map[string]interface{}{"match": "pattern", "key": key})
		}
		if result.Matches {
			result.Type = "pattern_" + result.Type
			return s.matchCheckResult(result)
		}
	}
	return module.CheckResult{}
}

var (
	senderHeaders    = []string{"Return-Path", "From", "Reply-To"}
	recipientHeaders = []string{"To", "Cc"}
)

// CheckBody checks the message headers against the pattern tables.
func (s *state) CheckBody(ctx context.Context, hdr textproto.Header, _ buffer.Buffer) module.CheckResult {
	ctx = context.WithValue(ctx, entrypointKey{}, "state.check-body")
	if s.msgMeta.Conn == nil {
		s.log.Msg("skipping locally generated message")
		return module.CheckResult{}
	}

	senderAddresses, err := getEmailAddresses(senderHeaders, hdr)
	if err != nil {
		return s.errorCheckResult(err, map[string]interface{}{"match": "sender"})
	}

	for senderAddress, headerName := range senderAddresses {
		result, err := s.c.checkEmailTable(ctx, s.c.matchSender, headerName, senderAddress, s.c.emailNorm)
		if err != nil {
			return s.errorCheckResult(err, map[string]interface{}{"match": "sender", "key": headerName})
		}
		if result.Matches {
			result.Type = "sender"
			return s.matchCheckResult(result)
		}

		if result := s.checkSenderAddressPattern(ctx, senderAddress); result.Reason != nil {
			return result
		}
	}

	recipientAddresses, err := getEmailAddresses(recipientHeaders, hdr)
	if err != nil {
		return s.errorCheckResult(err, map[string]interface{}{"match": "recipient"})
	}

	for recipientAddress, headerName := range recipientAddresses {
		result, err := s.c.checkEmailTable(ctx, s.c.matchRecipient, headerName, recipientAddress, s.c.emailNorm)
		if err != nil {
			return s.errorCheckResult(err, map[string]interface{}{"match": "recipient", "key": headerName})
		}
		if result.Matches {
			result.Type = "recipient"
			return s.matchCheckResult(result)
		}

		if result := s.checkRecipientAddressPattern(ctx, recipientAddress); result.Reason != nil {
			return result
		}

	}

	m, ok := s.c.match.(module.MultiTable)
	if ok {
		fields := hdr.Fields()
		for fields.Next() {
			result, err := checkPatternTable(ctx, m, s.c.reCache, fields.Key(), fields.Value(), s.c.headerNorm)
			if err != nil {
				return s.errorCheckResult(err, map[string]interface{}{"match": "pattern", "key": fields.Key()})
			}
			if result.Matches {
				result.Type = "pattern_" + result.Type
				return s.matchCheckResult(result)
			}
		}

	} else {
		s.log.DebugMsg("pattern match table is not a MultiTable")
	}

	return module.CheckResult{}

}

func (s *state) Close() error {
	return nil
}

func init() {
	module.Register(modName, New)
}
