// Package geobl implements a geoIP-based blocklist
//
//
// ## geobl check (check.geobl)
//
// The geobl module implements message filtering by looking up the remote SMTP
// server's IP address in a geoIP database.
//
// Example:
// ```
// check.geobl {
//	check_early yes
//	mmdb_pathname /var/lib/maddy/geoip.mmdb
//	blocklist_countries CA US
//	fail_action reject
// }
// ```
// ```
// check {
//     geobl { ... }
// }
// ```
//
// ## Configuration directives
//
// *Syntax*: check_early _boolean_ ++
// *Default*: no
//
// Check IP address before mail delivery starts and silently reject if sender
// is connecting from a country on the blocklist.
//
// *Syntax*: mmdb_pathname _pathname_ ++
//
// Path and filename to the MMDB country database to use for geoIP lookups.
//
// *Syntax*: block_countries _list_ ++
//
// List of two-character ISO3166-2 country codes to be blocked.
//
// *Syntax*: allow_countries _list_ ++
//
// List of two-character ISO3166-2 country codes to be allowed; all other
// country codes will be blocked. (Mutually-exclusive with block_countries.)
//
// *Syntax*: fail_action _action_ ++
// *Default*: quarantine
//
// Action to perform if the sender is connecting from a blocked country.
//
// *Syntax*: error_action _action_ ++
// *Default*: quarantine
//
// Action to perform if the IP address is not in the geoIP database or the
// geoIP database cannot be accessed.
//
package geobl
