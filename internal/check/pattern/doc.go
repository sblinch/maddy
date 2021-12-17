// Package pattern implements a simple pattern matching check
//
//
// ## pattern check (check.pattern)
//
// The pattern module implements message filtering by simple pattern matching
// against mail headers and connection metadata.
//
// Example:
// ```
// check.pattern {
// 	match_sender static {
// 		entry bad@example.org reject
// 		entry @suspicious.example.org quarantine
// 	}
// 	match_recipient static {
// 		entry honeypot@example.net quarantine
// 		entry @example.com reject
// 	}
// 	match_host static {
// 		entry 10. reject
// 		entry 192.168.0.1 reject
// 	}
// 	match file /etc/maddy/pattern.conf
// }
// ```
// ```
// check {
//     pattern { ... }
// }
// ```
//
// ## Configuration directives
//
// *Syntax:* email_normalize _action_
// *Default:* precis_casefold_email
//
// Normalization function to apply to email addresses before pattern matching.
// See `check.authorize_sender` documentation for available options.
//
// *Syntax:* header_normalize _action_
// *Default:* noop
//
// Normalization function to apply to message headers before pattern matching.
// See `check.authorize_sender` documentation for available options.
//
// *Syntax:* error_action _action_
//
// Action to perform if an error occurs during pattern handling.
//
// *Syntax:* match_sender _table_
//
// Table to use for sender address lookups, to be matched against the envelope
// sender (`MAIL FROM:`) as well as the `Return-Path:`, `From:`, and `Reply-To:`
// message headers. Key may be either a complete address (`foo@example.com`) or
// a domain name prefixed with `@` (`@domain.com`). Result of the lookup should
// be a valid action (`reject`, `quarantine`, or `ignore`) to be performed if
// the email address pattern matches.
//
// *Syntax:* match_recipient _table_
//
// Table to use for recipient address lookups, to be matched against the envelope
// recipient (`RCPT TO:`) as well as the `To:` and `Cc` message headers. Table
// definition is identical to `match_sender`.
//
// *Syntax:* match_host _table_
//
// Table to use for host IP address lookups, to be matched against the IP address
// of the remote SMTP server. Key may be either a complete IPv4/IPv6 address
// or one or more octets followed by a separator (eg: `127.` or `2001:db8:`).
// Result of the lookup should be a valid action (`reject`, `quarantine`, or
// `ignore`) to be performed if the email address pattern matches.
//
// For CIDR notation, use the `match` directive instead.
//
// *Syntax:* match _table_
//
// Table to use for arbitrary message header and connection metadata lookups.
// This must be a `table.file` or `table.sql_query` table as it requires a
// "multi" lookup. Key may be either a case-sensitive header name (eg: `Subject`)
// or one of a predefined set of connection metadata values (described below).
// Result of the lookup should be a string in the format `pattern action`,
// representing a pattern (defined below) and an action (`reject`, `quarantine`,
// or `ignore`) to be performed if the the value matches.
//
// Three pattern types are supported:
// 1. Keyword matching:
//     - `*keyword*` - matches `keyword` anywhere in the string
//     - `keyword*` - matches `keyword` at the beginning of the string
//     - `*keyword` - matches `keyword` at the end of the string
// 2. Regular expressions:
//     - `/sp[4a]m/` - matches the specified regular expression; the regular
//                     expression syntax is a subset of PCRE per
// 	                   https://golang.org/pkg/regexp/syntax/
// 3. CIDR notation:
//     - `cidr:10.5/16` - matches an IP address if it is contained within the
//                        specified network
//
// In addition to literal header names, the following additional connection
// metadata values are supported:
// - `helo-hostname` - the HELO/EHLO hostname provided by the remote SMTP server
// - `remote-addr` - the IP address of the remote SMTP server
// - `rdnsname` - the RDNS hostname for the remote SMTP server's IP address
// - `auth-user` - the authenticated username
// - `proto` - the connection protocol
// - `mail-from` - the envelope sender address
// - `rcpt-to` - the envelope recipient address
// - `sender-address` - matches against all sender addresses as per match_sender
// - `recipient-address` - matches against all recipient addresses as per match_recipient
//
//
// Examples:
// ```
// Subject: *pharmaceuticals* quarantine
// Subject: Spam* reject
// Date: *-0800 quarantine
// Received: "/from bad\.example\.(com|net|org)/" reject
// remote-addr: cidr:10.5.0.0/16 reject
// helo-hostname: *.example.org reject
// ```
package pattern
