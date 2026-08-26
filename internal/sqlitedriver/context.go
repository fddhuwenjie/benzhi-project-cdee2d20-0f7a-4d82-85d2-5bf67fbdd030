//go:build cgo

package sqlitedriver

/*
typedef struct sqlite3 sqlite3;
typedef struct sqlite3_stmt sqlite3_stmt;
int sqlite3_step(sqlite3_stmt*);
int sqlite3_finalize(sqlite3_stmt*);
int sqlite3_column_count(sqlite3_stmt*);
const char *sqlite3_column_name(sqlite3_stmt*, int);
int sqlite3_changes(sqlite3*);
long long sqlite3_last_insert_rowid(sqlite3*);
*/
import "C"

import (
	"context"
	"database/sql/driver"
	"errors"
	"unsafe"
)

func namedValues(values []driver.NamedValue) ([]driver.Value, error) {
	result := make([]driver.Value, len(values))
	for index, value := range values {
		if value.Name != "" {
			return nil, errors.New("SQLite 驱动不支持命名参数")
		}
		if !driver.IsValue(value.Value) {
			return nil, errors.New("SQLite 参数类型无效")
		}
		result[index] = value.Value
	}
	return result, nil
}

func (c *conn) Ping(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	rows, err := c.query("SELECT 1", nil)
	if err != nil {
		return err
	}
	return rows.Close()
}

func (c *conn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if options.ReadOnly {
		return nil, errors.New("SQLite 驱动不支持只读事务")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return c.begin()
}

func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	values, err := namedValues(args)
	if err != nil {
		return nil, err
	}
	return c.execute(query, values)
}

func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	values, err := namedValues(args)
	if err != nil {
		return nil, err
	}
	return c.query(query, values)
}

func (c *conn) execute(query string, values []driver.Value) (driver.Result, error) {
	statement, err := c.prepare(query, values)
	if err != nil {
		return nil, err
	}
	code := C.sqlite3_step(statement)
	if code != sqliteDone && code != sqliteRow {
		message := c.databaseError(code)
		C.sqlite3_finalize(statement)
		return nil, message
	}
	for code == sqliteRow {
		code = C.sqlite3_step(statement)
	}
	if code != sqliteDone {
		message := c.databaseError(code)
		C.sqlite3_finalize(statement)
		return nil, message
	}
	lastID := int64(C.sqlite3_last_insert_rowid(c.db))
	changed := int64(C.sqlite3_changes(c.db))
	if finalCode := C.sqlite3_finalize(statement); finalCode != sqliteOK {
		return nil, c.databaseError(finalCode)
	}
	return result{lastInsertID: lastID, rowsAffected: changed}, nil
}

func (c *conn) query(query string, values []driver.Value) (driver.Rows, error) {
	statement, err := c.prepare(query, values)
	if err != nil {
		return nil, err
	}
	count := int(C.sqlite3_column_count(statement))
	columns := make([]string, count)
	for index := range columns {
		columns[index] = C.GoString(C.sqlite3_column_name(statement, C.int(index)))
	}
	return &rows{conn: c, statement: statement, columns: columns}, nil
}

var _ driver.Driver = (*Driver)(nil)
var _ driver.Conn = (*conn)(nil)
var _ driver.ConnBeginTx = (*conn)(nil)
var _ driver.ExecerContext = (*conn)(nil)
var _ driver.QueryerContext = (*conn)(nil)
var _ driver.Pinger = (*conn)(nil)
var _ = unsafe.Pointer(nil)
