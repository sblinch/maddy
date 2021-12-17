// Package spamassassin implements a SpamAssassin check
//
//
// ## spamassassin check (check.spamassassin)
//
// The spamassassin module implements message filtering by contacting a
// spamassassin "spamd" server.
//
// ```
// check.spamassassin {
// 	address tcp://127.0.0.1:783
// 	spamd_user_type unix
//
// 	io_error_action ignore
// 	error_resp_action ignore
// 	spam_action quarantine
// }
// ```
//
// ## Configuration directives
//
// *Syntax:* address { ... } ++
// *Default:* tcp://127.0.0.1:783
//
// URL of spamd server. Supports "tcp", "tls", and "unix" protocols.
//
// *Syntax:* spamd_user_type _type_ ++
// *Default:* unix
//
// Specifies the type of username to pass to SpamAssassin.
//
// `unix` passes the value of `spamd_user`.
//
// `email` passes the recipient email address.
//
// `username` passes the username portion of the recipient email address.
//
// *Syntax:* spamd_user _username_
// *Default:* the username of the UNIX user account under which Maddy is running
//
// Specifies the a username to pass to SpamAssassin when `spamd_user_type` is
// `unix`.
//
// *Syntax:* insecure_tls
// *Default:* no
//
// Do not verify the peer certificate when connecting to SpamAssassin using TLS.
//
// *Syntax:* compression
// *Default:* no
//
// Compress messages before sending them to SpamAssassin.
//
// *Syntax:* io_error_action _action_ ++
// *Default:* ignore
//
// Action to take in case of inability to contact the SpamAssassin server.
//
// *Syntax:* error_resp_action _action_ ++
// *Default:* ignore
//
// Action to take in case of an error response from the SpamAssassin server.
//
// *Syntax:* spam_action _action_ ++
// *Default:* quarantine
//
// Action to take when SpamAssassin determines that the message is spam, per its
// configured score thresholds.
//
// *Syntax:* quarantine_threshold _score_ ++
// *Default:* 0.0
//
// Spam score threshold at which to quarantine a message. Typically this is used
// with `spam_action ignore` to implement custom scoring independent of the
// SpamAssassin threshold configuration.
//
// Use 0.0 to disable.
//
// *Syntax:* reject_threshold _score_ ++
// *Default:* 0.0
//
// Spam score threshold at which to reject a message. Typically this is used
// with `spam_action ignore` to implement custom scoring independent of the
// SpamAssassin threshold configuration.
//
// Use 0.0 to disable.
//
package spamassassin
