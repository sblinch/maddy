// Package table implements blob storage in a Maddy table.
//
//
// # Table storage (storage.blob.table)
//
// This module stores message bodies in any mutable table module (eg: sql_table)
// supported by Maddy.
//
// ```
// storage.blob.table {
// 	chunk_size 1048576
// 	table sql_table {
// 		driver sqlite3
// 		dsn messages.db
// 		table_name messages
// 	}
// }
// ```
//
// ## Configuration directives
//
// *Syntax:* chunk_size _size_ ++
// *Default:* 1048576
//
// Size of chunks written to the table. Each chunk needs to be stored in memory
// during processing, so this also represents the size of the memory buffer
// allocated for each delivery.
//
// *Syntax:* table *table* ++
//
// Use specified table module (*maddy-tables*(5)) for backend storage. Note that
// this must be a *mutable* table; currently sql_table is the only mutable
// table format supported by Maddy.
//
package table
