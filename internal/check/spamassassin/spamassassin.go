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

package spamassassin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	spamc "github.com/baruwa-enterprise/spamd-client/pkg"
	saresponse "github.com/baruwa-enterprise/spamd-client/pkg/response"
	"github.com/emersion/go-message/textproto"
	"github.com/foxcpp/maddy/framework/address"
	"github.com/foxcpp/maddy/framework/buffer"
	"github.com/foxcpp/maddy/framework/config"
	modconfig "github.com/foxcpp/maddy/framework/config/module"
	"github.com/foxcpp/maddy/framework/exterrors"
	"github.com/foxcpp/maddy/framework/log"
	"github.com/foxcpp/maddy/framework/module"
	"github.com/foxcpp/maddy/internal/target"
)

const modName = "check.spamassassin"

type Check struct {
	instName string
	log      log.Logger

	address       string
	spamdUser     string
	spamdUserType string

	quarantineThreshold float64
	rejectThreshold     float64

	ioErrAction     modconfig.FailAction
	errorRespAction modconfig.FailAction
	spamAction      modconfig.FailAction
	/*
		rewriteSubjAction modconfig.FailAction

	*/

	clientPool sync.Pool
}

func New(modName, instName string, _, inlineArgs []string) (module.Module, error) {
	c := &Check{
		instName: instName,
		log:      log.Logger{Name: modName, Debug: log.DefaultLogger.Debug},
	}

	switch len(inlineArgs) {
	case 1:

		c.address = inlineArgs[0]
	case 0:
		c.address = "127.0.0.1:783"
	default:
		return nil, fmt.Errorf("%s: unexpected amount of inline arguments", modName)
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
	var (
		insecureTLS bool
		compression bool
		connTimeout time.Duration
		cmdTimeout  time.Duration
	)

	// enable debug logging
	cfg.Bool("debug", false, false, &c.log.Debug)
	// SpamAssassin server address: tls://host:port, tcp://host:port, unix:///foo/bar.sock
	cfg.String("address", false, false, c.address, &c.address)
	// disable peer TLS certificate verification
	cfg.Bool("insecure_tls", false, false, &insecureTLS)
	// compress messages before sending to SA
	cfg.Bool("compression", false, false, &compression)
	// username for SA (for per-user Bayes/prefs/etc), or blank to use the user under which Maddy is running
	cfg.String("spamd_user", false, false, c.spamdUser, &c.spamdUser)
	// unix = use spamd_user; username = use the username portion of the recipient address; email = use the entire recipient address
	cfg.Enum("spamd_user_type", false, false, []string{"unix", "username", "email"}, "unix", &c.spamdUserType)
	// timeout for connecting to the SA server
	cfg.Duration("connect_timeout", false, false, 3*time.Second, &connTimeout)
	// maximum time for SA to process a message and return a result
	cfg.Duration("command_timeout", false, false, 8*time.Second, &cmdTimeout)

	// by default, we let SpamAssassin's configuration decide the threshold and we simply perform spam_action (below)
	// when SA decides the message is spam; alternately, specify `spam_action ignore` and set these thresholds instead
	cfg.Float("quarantine_threshold", false, false, 0, &c.quarantineThreshold)
	cfg.Float("reject_threshold", false, false, 0, &c.rejectThreshold)

	// action to perform on error in connecting to SA
	cfg.Custom("io_error_action", false, false,
		func() (interface{}, error) {
			return modconfig.FailAction{}, nil
		}, modconfig.FailActionDirective, &c.ioErrAction)
	// action to perform when SA returns an error
	cfg.Custom("error_resp_action", false, false,
		func() (interface{}, error) {
			return modconfig.FailAction{}, nil
		}, modconfig.FailActionDirective, &c.errorRespAction)
	// action to perform when SA decides the message is spam
	cfg.Custom("spam_action", false, false,
		func() (interface{}, error) {
			return modconfig.FailAction{Quarantine: true}, nil
		}, modconfig.FailActionDirective, &c.spamAction)
	if _, err := cfg.Process(); err != nil {
		return err
	}

	u, err := url.Parse(c.address)
	if err != nil {
		return fmt.Errorf("%s: %s", modName, err)
	}

	if c.spamdUser == "" {
		u, err := user.Current()
		if err != nil {
			return fmt.Errorf("%s: cannot get current user", modName)
		}
		c.spamdUser = u.Username
	}

	network := "tcp"
	spamdaddress := ""
	useTLS := false

	switch u.Scheme {
	case "unix":
		network = "unix"
		spamdaddress = u.Path
	case "tls":
		useTLS = true
		spamdaddress = u.Host
	case "tcp", "":
		spamdaddress = u.Host
	default:
		return fmt.Errorf("%s: invalid address scheme", modName)
	}

	_, err = spamc.NewClient(network, spamdaddress, "", compression)
	if err != nil {
		return fmt.Errorf("%s: %s", modName, err)
	}

	c.clientPool.New = func() interface{} {
		cli, err := spamc.NewClient(network, spamdaddress, "", compression)
		if err == nil {
			if useTLS {
				cli.EnableTLS()
			}
			if insecureTLS {
				cli.DisableTLSVerification()
			}
			cli.SetConnTimeout(connTimeout)
			cli.SetCmdTimeout(cmdTimeout)
			cli.SetConnRetries(0)
			cli.SetConnSleep(0)
		} else {
			cli = nil
		}
		return cli
	}

	return nil
}

type state struct {
	c       *Check
	msgMeta *module.MsgMetadata
	log     log.Logger

	mailFrom string
	rcpt     []string
}

func (c *Check) CheckStateForMsg(ctx context.Context, msgMeta *module.MsgMetadata) (module.CheckState, error) {
	return &state{
		c:       c,
		msgMeta: msgMeta,
		log:     target.DeliveryLogger(c.log, msgMeta),
	}, nil
}

func (c *Check) buildAddedHeaders(rs *saresponse.Response) textproto.Header {
	hdrAdd := textproto.Header{}
	for name, values := range rs.Headers {
		for _, value := range values {
			hdrAdd.Add("X-"+name, value)
		}
	}

	isSpam := "No"
	if rs.IsSpam {
		isSpam = "Yes"
	}
	hdrAdd.Set("X-Spam-Flag", isSpam)
	hdrAdd.Set("X-Spam-Score", strconv.FormatFloat(rs.Score, 'f', 2, 64))

	status := strings.Builder{}
	status.WriteString(isSpam)
	status.WriteString(", score=")
	status.WriteString(strconv.FormatFloat(rs.Score, 'f', 2, 64))
	if len(rs.Rules) > 0 {
		status.WriteString(" tests=")
		for n, rules := range rs.Rules {
			if n > 0 {
				status.WriteByte(' ')
			}
			status.WriteString(rules["name"])
			status.WriteByte('=')
			status.WriteString(rules["score"])
		}
	}
	if rs.Version != "" {
		status.WriteString(" version=spamassassin ")
		status.WriteString(rs.Version)
	}
	hdrAdd.Set("X-Spam-Status", status.String())

	return hdrAdd
}

func (s *state) CheckConnection(ctx context.Context) module.CheckResult {
	return module.CheckResult{}
}

func (s *state) CheckSender(ctx context.Context, addr string) module.CheckResult {
	s.mailFrom = addr
	return module.CheckResult{}
}

func (s *state) CheckRcpt(ctx context.Context, addr string) module.CheckResult {
	s.rcpt = append(s.rcpt, addr)
	return module.CheckResult{}
}

type multiReaderLen struct {
	io.Reader
	combinedLen int
}

func (r multiReaderLen) Len() int {
	return r.combinedLen
}

func (s *state) getSpamdUser() (string, error) {
	spamdUser := s.c.spamdUser
	if len(s.rcpt) == 1 {
		switch s.c.spamdUserType {
		case "unix":
			// use as-is
		case "username":
			username, _, err := address.Split(s.rcpt[0])
			if err != nil {
				return "", err
			}
			spamdUser = username
		case "email":
			spamdUser = s.rcpt[0]
		}
	}
	return spamdUser, nil
}

func (s *state) CheckBody(ctx context.Context, hdr textproto.Header, body buffer.Buffer) module.CheckResult {
	bodyR, err := body.Open()

	var buf bytes.Buffer
	if err == nil {
		err = textproto.WriteHeader(&buf, hdr)
	}

	if err != nil {
		return s.c.ioErrAction.Apply(module.CheckResult{
			Reason: &exterrors.SMTPError{
				Code:         451,
				EnhancedCode: exterrors.EnhancedCode{4, 7, 0},
				Message:      "Internal error during policy check",
				CheckName:    modName,
				Err:          err,
			},
		})
	}

	c := s.c.clientPool.Get().(*spamc.Client)
	defer s.c.clientPool.Put(c)

	spamdUser, err := s.getSpamdUser()
	if err != nil {
		return s.c.ioErrAction.Apply(module.CheckResult{
			Reason: &exterrors.SMTPError{
				Code:         451,
				EnhancedCode: exterrors.EnhancedCode{4, 7, 0},
				Message:      "Internal error during policy check",
				CheckName:    modName,
				Err:          err,
			},
		})
	}
	c.SetUser(spamdUser)

	mrl := multiReaderLen{
		Reader:      io.MultiReader(&buf, bodyR),
		combinedLen: buf.Len() + body.Len(),
	}

	rs, err := c.Check(ctx, mrl)
	if err != nil {
		return s.c.ioErrAction.Apply(module.CheckResult{
			Reason: &exterrors.SMTPError{
				Code:         451,
				EnhancedCode: exterrors.EnhancedCode{4, 7, 0},
				Message:      "Internal error during policy check",
				CheckName:    modName,
				Err:          err,
			},
		})
	}

	if rs.StatusCode != saresponse.ExOK {
		return s.c.errorRespAction.Apply(module.CheckResult{
			Reason: &exterrors.SMTPError{
				Code:         451,
				EnhancedCode: exterrors.EnhancedCode{4, 7, 0},
				Message:      "Internal error during policy check",
				CheckName:    modName,
				Err:          rs.StatusCode,
			},
		})
	}

	hdrAdd := s.c.buildAddedHeaders(rs)

	action := modconfig.FailAction{}
	reason := ""
	var misc map[string]interface{}

	if rs.IsSpam && (s.c.spamAction.Quarantine || s.c.spamAction.Reject) {
		action = s.c.spamAction
		reason = "message is spam"
		misc = map[string]interface{}{"spam-score": rs.Score}
	} else if s.c.rejectThreshold >= 0.001 && rs.Score >= s.c.rejectThreshold {
		action.Reject = true
		reason = "spam score exceeds reject threshold"
		misc = map[string]interface{}{"spam-score": rs.Score, "spam-reject-threshold": s.c.rejectThreshold}
	} else if s.c.quarantineThreshold >= 0.001 && rs.Score >= s.c.quarantineThreshold {
		action.Quarantine = true
		reason = "spam score exceeds quarantine threshold"
		misc = map[string]interface{}{"spam-score": rs.Score, "spam-quarantine-threshold": s.c.quarantineThreshold}
	} else {
		return module.CheckResult{
			Header: hdrAdd,
		}
	}

	return action.Apply(module.CheckResult{
		Reason: &exterrors.SMTPError{
			Code:         550,
			EnhancedCode: exterrors.EnhancedCode{5, 7, 0},
			Message:      "Message rejected due to local policy",
			CheckName:    modName,
			Reason:       reason,
			Misc:         misc,
		},
		Header: hdrAdd,
	})
}

func (s *state) Close() error {
	return nil
}

func init() {
	module.Register(modName, New)
}
