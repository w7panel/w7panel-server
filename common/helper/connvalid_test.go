package helper

// The former DSN ping tests are disabled: they attempted to connect to fixed
// private MySQL and PostgreSQL addresses with hard-coded credentials. DBConnTest
// has no injectable connector, so these cannot be deterministic unit tests.
