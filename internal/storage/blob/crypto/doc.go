// Package crypto implements encrypted blob storage
//
//
// # Crypto storage (storage.blob.crypto)
//
// This module can be used to add encryption support to any other blob storage
// module. The underlying blob storage module never sees the plaintext version
// of the message content.
//
// Data is encrypted in the Data At Rest Encryption (DARE) format with the
// AES-256-GCM cipher (when hardware AES support is available) or the
// ChaCha20-Poly1305 cipher when not, using MinIO's Secure IO package.
//
// ```
// storage.blob.crypto {
// 	msg_store fs messages
//	crypto_static_key "RG9uJ3QgYWN0dWFsbHkgdXNlIGEgcGFzc3BocmFzZS4="
// }
// ```
//
// ## Configuration directives
//
// *Syntax*: msg_store _store_ ++
// *Default*: fs messages/
//
// Module to use for actual storage of encrypted message bodies. This module
// receives an encrypted blob and never sees the plaintext message body content.
//
// See *maddy-blob*(5) for details.
//
// *Syntax:* crypto_static_key _key_ ++
//
// Key used to encrypt and decrypt stored blobs. This must be a bae64-encoded
// string that decodes to precisely 32 bytes of random data. This can be
// generated using:
// ```
// head -c 32 /dev/urandom | base64
// ```
package crypto
