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

package geobl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime/trace"

	"github.com/IncSW/geoip2"
	"github.com/emersion/go-message/textproto"
	"github.com/emersion/go-smtp"
	"github.com/foxcpp/maddy/framework/buffer"
	"github.com/foxcpp/maddy/framework/config"
	modconfig "github.com/foxcpp/maddy/framework/config/module"
	"github.com/foxcpp/maddy/framework/exterrors"
	"github.com/foxcpp/maddy/framework/log"
	"github.com/foxcpp/maddy/framework/module"
	"github.com/foxcpp/maddy/internal/target"
)

const modName = "geobl"

type GeoBL struct {
	instName string
	modName  string
	log      log.Logger

	checkEarly     bool
	failOpen       bool
	mmdbPath       string
	blockCountries map[string]struct{}
	allowCountries map[string]struct{}
	failAction     modconfig.FailAction
	errorAction    modconfig.FailAction

	geoipReader *geoip2.CountryReader
}

func New(modName, instName string, aliases, inlineArgs []string) (module.Module, error) {
	g := &GeoBL{
		instName: instName,
		modName:  modName,
		log:      log.Logger{Name: modName, Debug: log.DefaultLogger.Debug},
	}

	switch len(inlineArgs) {
	case 1:
		g.mmdbPath = inlineArgs[0]
	case 0:
	default:
		return nil, fmt.Errorf("%s: unexpected amount of inline arguments", modName)
	}

	return g, nil
}

func (g *GeoBL) Name() string {
	return modName
}

func (g *GeoBL) InstanceName() string {
	return g.instName
}

func (g *GeoBL) Init(cfg *config.Map) error {
	blockCountries, allowCountries := []string{}, []string{}
	cfg.Bool("debug", false, false, &g.log.Debug)
	cfg.String("mmdb_pathname", true, true, g.mmdbPath, &g.mmdbPath)
	cfg.StringList("allow_countries", true, false, []string{}, &allowCountries)
	cfg.StringList("block_countries", true, false, []string{}, &blockCountries)
	cfg.Bool("check_early", true, false, &g.checkEarly)
	cfg.Custom("error_action", false, false,
		func() (interface{}, error) {
			return modconfig.FailAction{Quarantine: true}, nil
		}, modconfig.FailActionDirective, &g.errorAction)

	cfg.Custom("fail_action", false, false,
		func() (interface{}, error) {
			return modconfig.FailAction{Quarantine: true}, nil
		}, modconfig.FailActionDirective, &g.failAction)

	if _, err := cfg.Process(); err != nil {
		return err
	}

	if len(blockCountries) > 0 {
		if len(allowCountries) > 0 {
			return fmt.Errorf("%s: cannot specify both a block and allow list", g.modName)
		}
		g.blockCountries = make(map[string]struct{}, len(blockCountries))
		for _, country := range blockCountries {
			g.blockCountries[country] = struct{}{}
		}
		g.log.DebugMsg("blocked countries", "codes", blockCountries)

	} else if len(allowCountries) > 0 {
		g.allowCountries = make(map[string]struct{}, len(allowCountries))
		for _, country := range allowCountries {
			g.allowCountries[country] = struct{}{}
		}
		g.log.DebugMsg("allowed countries", "codes", allowCountries)
	} else {
		return fmt.Errorf("%s: must specify a block or allow list", g.modName)
	}

	var err error
	if g.geoipReader, err = geoip2.NewCountryReaderFromFile(g.mmdbPath); err != nil {
		return fmt.Errorf("%s: failed to initialize MMDB file: %v", g.modName, err)
	}

	return nil
}

type state struct {
	g       *GeoBL
	msgMeta *module.MsgMetadata
	log     log.Logger
}

var (
	errCountryUnknown    = errors.New("IP country is unknown")
	errCountryBlocked    = errors.New("client is connecting from a blocked country")
	errCountryNotAllowed = errors.New("client is not connecting from an allowed country")
)

func (g *GeoBL) checkIP(ip net.IP) module.CheckResult {
	result, err := g.geoipReader.Lookup(ip)

	if err == nil && (result.Country.ISOCode == "Unknown" || result.Country.ISOCode == "None") {
		err = errCountryUnknown
	}

	if err != nil {
		g.log.DebugMsg("error looking up sender country", "error", err.Error())
		return g.errorAction.Apply(module.CheckResult{
			Reason: &exterrors.SMTPError{
				Code:         554,
				EnhancedCode: exterrors.EnhancedCode{0, 7, 0},
				Message:      "Error during policy check",
				Err:          err,
				CheckName:    modName,
				Misc:         map[string]interface{}{"geobl-address": ip.String()},
			},
		})
	}

	if g.blockCountries != nil {
		if _, blocked := g.blockCountries[result.Country.ISOCode]; blocked {
			g.log.DebugMsg("sender country is blocked", "country", result.Country.ISOCode)
			err = errCountryBlocked
		}
	} else {
		if _, allowed := g.allowCountries[result.Country.ISOCode]; !allowed {
			g.log.DebugMsg("sender country is not allowed", "country", result.Country.ISOCode)
			err = errCountryNotAllowed
		}
	}

	if err != nil {
		return g.failAction.Apply(module.CheckResult{
			Reason: &exterrors.SMTPError{
				Code:         554,
				EnhancedCode: exterrors.EnhancedCode{5, 7, 0},
				Err:          err,
				CheckName:    modName,
				Misc:         map[string]interface{}{"geobl-country": result.Country.ISOCode, "geobl-address": ip.String()},
			},
		})
	}

	g.log.DebugMsg("sender country is permitted", "country", result.Country.ISOCode)

	return module.CheckResult{}
}

// CheckConnection implements module.EarlyCheck.
func (g *GeoBL) CheckConnection(ctx context.Context, state *smtp.ConnectionState) error {
	if !g.checkEarly {
		return nil
	}

	defer trace.StartRegion(ctx, "geobl/CheckConnection (Early)").End()

	ip, ok := state.RemoteAddr.(*net.TCPAddr)
	if !ok {
		g.log.Msg("non-TCP/IP source", "src_addr", state.RemoteAddr, "src_host", state.Hostname)
		return nil
	}

	result := g.checkIP(ip.IP)
	if result.Reject {
		return result.Reason
	}

	return nil
}

func (g *GeoBL) CheckStateForMsg(ctx context.Context, msgMeta *module.MsgMetadata) (module.CheckState, error) {
	return &state{
		g:       g,
		msgMeta: msgMeta,
		log:     target.DeliveryLogger(g.log, msgMeta),
	}, nil
}

func (s *state) CheckConnection(ctx context.Context) module.CheckResult {
	if s.g.checkEarly {
		// Already checked before.
		return module.CheckResult{}
	}

	defer trace.StartRegion(ctx, "geobl/CheckConnection").End()

	if s.msgMeta.Conn == nil {
		s.log.Msg("locally generated message, ignoring")
		return module.CheckResult{}
	}

	ip, ok := s.msgMeta.Conn.RemoteAddr.(*net.TCPAddr)
	if !ok {
		s.log.Msg("non-TCP/IP source")
		return module.CheckResult{}
	}

	return s.g.checkIP(ip.IP)
}

func (s *state) CheckSender(ctx context.Context, addr string) module.CheckResult {
	return module.CheckResult{}
}

func (s *state) CheckRcpt(ctx context.Context, addr string) module.CheckResult {
	return module.CheckResult{}
}

func (s *state) CheckBody(ctx context.Context, hdr textproto.Header, body buffer.Buffer) module.CheckResult {
	return module.CheckResult{}
}

func (s *state) Close() error {
	return nil
}

func init() {
	module.Register(modName, New)
}
