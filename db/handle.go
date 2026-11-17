package db

// Handle is the connection or transaction q runs on, for the few places that need SQL no query here
// states (such as a Numbering that counts by its own scheme and locks while it does).
func (q *Queries) Handle() DBTX { return q.db }
