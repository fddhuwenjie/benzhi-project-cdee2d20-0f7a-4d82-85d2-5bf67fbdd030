//go:build !cgo

// Package sqlitedriver provides a small fallback for environments where CGO
// is disabled. The production SQLite implementation is compiled when CGO is
// available; this fallback keeps cross-compilation and static builds
// well-defined while returning a clear error if a database connection is
// attempted at runtime.
package sqlitedriver

import (
	"database/sql"
	"database/sql/driver"
	"errors"
)

var errCGODisabled = errors.New("SQLite 驱动需要启用 CGO")

// Driver implements database/sql/driver.Driver for CGO-disabled builds.
// SQLite operations cannot be performed without the native SQLite library,
// so Open fails explicitly instead of silently providing non-persistent data.
type Driver struct{}

func init() { sql.Register("sqlite", &Driver{}) }

func (d *Driver) Open(string) (driver.Conn, error) { return nil, errCGODisabled }
