// Package domainbl implements a domain blocklist
//
//
// ## domainbl check (check.domainbl)
//
// The domainbl module implements message filtering by looking up domain names
// found in the message body on domain blocklists such as URIBL or SURBL.
//
// Example:
// ```
// check.domainbl {
//	quarantine_threshold 1
//	reject_threshold 1
//	domainbl.example.org {
//		bits 8+16+64
//		score 1
//	}
// }
// ```
// ```
// check {
//     domainbl { ... }
// }
// ```
//
// ## Configuration directives
//
// *Syntax*: quarantine_threshold _integer_ ++
// *Default*: 1
//
// domainbl score needed (equals-or-higher) to quarantine the message.
//
// *Syntax*: reject_threshold _integer_ ++
// *Default*: 9999
//
// domainbl score needed (equals-or-higher) to reject the message.
//
// ## List configuration
//
// ```
// domainbl.example.org {
//	bits 8+16+64
//	score 1
// }
// ```
//
// Directive name specifies the actual DNS zone to query when checking
// the list.
//
// *Syntax*: bits _bits_ ++
//
// Bits positions to match in the last octet of the domainbl response address
// (eg: a response of `127.0.0.10` represents bit positions 2 and 8).
// May be specified as a numeric value (eg: `10` for bit positions 2 and 8) or
// as a list of bit positions separated by plus symbols (eg: `2+8`).
//
// *Syntax*: score _integer_ ++
// *Default*: 1
//
// Score value to add for the message if it is listed.
//
//
package domainbl
