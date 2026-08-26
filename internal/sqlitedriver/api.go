//go:build cgo

package sqlitedriver

/*
#cgo LDFLAGS: -l:libsqlite3.so.0
#include <stdlib.h>

typedef struct sqlite3 sqlite3;
typedef struct sqlite3_stmt sqlite3_stmt;
typedef long long sqlite3_int64;

int sqlite3_open_v2(const char*, sqlite3**, int, const char*);
int sqlite3_close_v2(sqlite3*);
const char *sqlite3_errmsg(sqlite3*);
int sqlite3_prepare_v2(sqlite3*, const char*, int, sqlite3_stmt**, const char**);
int sqlite3_step(sqlite3_stmt*);
int sqlite3_finalize(sqlite3_stmt*);
int sqlite3_reset(sqlite3_stmt*);
int sqlite3_clear_bindings(sqlite3_stmt*);
int sqlite3_bind_parameter_count(sqlite3_stmt*);
int sqlite3_bind_null(sqlite3_stmt*, int);
int sqlite3_bind_int64(sqlite3_stmt*, int, sqlite3_int64);
int sqlite3_bind_double(sqlite3_stmt*, int, double);
int sqlite3_bind_text(sqlite3_stmt*, int, const char*, int, void(*)(void*));
int sqlite3_bind_blob(sqlite3_stmt*, int, const void*, int, void(*)(void*));
int sqlite3_column_count(sqlite3_stmt*);
const char *sqlite3_column_name(sqlite3_stmt*, int);
int sqlite3_column_type(sqlite3_stmt*, int);
sqlite3_int64 sqlite3_column_int64(sqlite3_stmt*, int);
double sqlite3_column_double(sqlite3_stmt*, int);
const unsigned char *sqlite3_column_text(sqlite3_stmt*, int);
const void *sqlite3_column_blob(sqlite3_stmt*, int);
int sqlite3_column_bytes(sqlite3_stmt*, int);
int sqlite3_changes(sqlite3*);
sqlite3_int64 sqlite3_last_insert_rowid(sqlite3*);
int sqlite3_busy_timeout(sqlite3*, int);

static int bind_text_transient(sqlite3_stmt *s, int i, const char *v, int n) {
    return sqlite3_bind_text(s, i, v, n, (void(*)(void*))-1);
}
static int bind_blob_transient(sqlite3_stmt *s, int i, const void *v, int n) {
    return sqlite3_bind_blob(s, i, v, n, (void(*)(void*))-1);
}
*/
import "C"

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"
)

const (
	sqliteOK      = 0
	sqliteRow     = 100
	sqliteDone    = 101
	sqliteInteger = 1
	sqliteFloat   = 2
	sqliteText    = 3
	sqliteBlob    = 4
	sqliteNull    = 5
	openReadWrite = 0x00000002
	openCreate    = 0x00000004
	openURI       = 0x00000040
	openFullMutex = 0x00010000
)

func init() { sql.Register("sqlite", &Driver{}) }

type Driver struct{}

type conn struct {
	db     *C.sqlite3
	mu     sync.Mutex
	closed bool
}

type stmt struct {
	conn  *conn
	query string
}

type tx struct {
	conn *conn
	done bool
}

type result struct {
	lastInsertID int64
	rowsAffected int64
}

type rows struct {
	conn      *conn
	statement *C.sqlite3_stmt
	columns   []string
	closed    bool
}

func (d *Driver) Open(name string) (driver.Conn, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	var db *C.sqlite3
	code := C.sqlite3_open_v2(cName, &db, openReadWrite|openCreate|openURI|openFullMutex, nil)
	if code != sqliteOK {
		message := "无法打开 SQLite"
		if db != nil {
			message = C.GoString(C.sqlite3_errmsg(db))
			C.sqlite3_close_v2(db)
		}
		return nil, errors.New(message)
	}
	C.sqlite3_busy_timeout(db, 5000)
	connection := &conn{db: db}
	if _, err := connection.execute("PRAGMA foreign_keys=ON", nil); err != nil {
		C.sqlite3_close_v2(db)
		return nil, err
	}
	if _, err := connection.execute("PRAGMA journal_mode=WAL", nil); err != nil {
		C.sqlite3_close_v2(db)
		return nil, err
	}
	return connection, nil
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	if c.closed {
		return nil, driver.ErrBadConn
	}
	return &stmt{conn: c, query: query}, nil
}

func (c *conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	if code := C.sqlite3_close_v2(c.db); code != sqliteOK {
		return c.databaseError(code)
	}
	c.closed = true
	return nil
}

func (c *conn) Begin() (driver.Tx, error) { return c.begin() }

func (c *conn) begin() (driver.Tx, error) {
	if _, err := c.execute("BEGIN IMMEDIATE", nil); err != nil {
		return nil, err
	}
	return &tx{conn: c}, nil
}

func (c *conn) prepare(query string, values []driver.Value) (*C.sqlite3_stmt, error) {
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))
	var statement *C.sqlite3_stmt
	code := C.sqlite3_prepare_v2(c.db, cQuery, -1, &statement, nil)
	if code != sqliteOK {
		return nil, c.databaseError(code)
	}
	if count := int(C.sqlite3_bind_parameter_count(statement)); count != len(values) {
		C.sqlite3_finalize(statement)
		return nil, fmt.Errorf("SQLite 参数数量不匹配: 需要 %d，实际 %d", count, len(values))
	}
	for index, value := range values {
		if err := bind(statement, index+1, value); err != nil {
			C.sqlite3_finalize(statement)
			return nil, err
		}
	}
	return statement, nil
}

func bind(statement *C.sqlite3_stmt, index int, value driver.Value) error {
	var code C.int
	switch typed := value.(type) {
	case nil:
		code = C.sqlite3_bind_null(statement, C.int(index))
	case int64:
		code = C.sqlite3_bind_int64(statement, C.int(index), C.sqlite3_int64(typed))
	case float64:
		code = C.sqlite3_bind_double(statement, C.int(index), C.double(typed))
	case bool:
		integer := int64(0)
		if typed {
			integer = 1
		}
		code = C.sqlite3_bind_int64(statement, C.int(index), C.sqlite3_int64(integer))
	case string:
		pointer := C.CString(typed)
		defer C.free(unsafe.Pointer(pointer))
		code = C.bind_text_transient(statement, C.int(index), pointer, C.int(len(typed)))
	case []byte:
		if len(typed) == 0 {
			code = C.bind_blob_transient(statement, C.int(index), nil, 0)
		} else {
			code = C.bind_blob_transient(statement, C.int(index), unsafe.Pointer(&typed[0]), C.int(len(typed)))
		}
	default:
		return fmt.Errorf("SQLite 不支持参数类型 %T", value)
	}
	if code != sqliteOK {
		return fmt.Errorf("SQLite 绑定参数失败: %d", code)
	}
	return nil
}

func (c *conn) databaseError(code C.int) error {
	return fmt.Errorf("SQLite 错误 %d: %s", int(code), C.GoString(C.sqlite3_errmsg(c.db)))
}

func (s *stmt) Close() error                                    { return nil }
func (s *stmt) NumInput() int                                   { return -1 }
func (s *stmt) Exec(args []driver.Value) (driver.Result, error) { return s.conn.execute(s.query, args) }
func (s *stmt) Query(args []driver.Value) (driver.Rows, error)  { return s.conn.query(s.query, args) }

func (t *tx) Commit() error {
	if t.done {
		return errors.New("事务已经结束")
	}
	_, err := t.conn.execute("COMMIT", nil)
	if err == nil {
		t.done = true
	}
	return err
}

func (t *tx) Rollback() error {
	if t.done {
		return errors.New("事务已经结束")
	}
	_, err := t.conn.execute("ROLLBACK", nil)
	if err == nil {
		t.done = true
	}
	return err
}

func (r result) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r result) RowsAffected() (int64, error) { return r.rowsAffected, nil }

func (r *rows) Columns() []string { return append([]string(nil), r.columns...) }

func (r *rows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if code := C.sqlite3_finalize(r.statement); code != sqliteOK {
		return r.conn.databaseError(code)
	}
	return nil
}

func (r *rows) Next(destination []driver.Value) error {
	if r.closed {
		return io.EOF
	}
	code := C.sqlite3_step(r.statement)
	if code == sqliteDone {
		return io.EOF
	}
	if code != sqliteRow {
		return r.conn.databaseError(code)
	}
	for index := range destination {
		destination[index] = columnValue(r.statement, index)
	}
	return nil
}

func columnValue(statement *C.sqlite3_stmt, index int) driver.Value {
	switch int(C.sqlite3_column_type(statement, C.int(index))) {
	case sqliteInteger:
		return int64(C.sqlite3_column_int64(statement, C.int(index)))
	case sqliteFloat:
		return float64(C.sqlite3_column_double(statement, C.int(index)))
	case sqliteText:
		pointer := C.sqlite3_column_text(statement, C.int(index))
		length := C.sqlite3_column_bytes(statement, C.int(index))
		return C.GoStringN((*C.char)(unsafe.Pointer(pointer)), length)
	case sqliteBlob:
		pointer := C.sqlite3_column_blob(statement, C.int(index))
		length := C.sqlite3_column_bytes(statement, C.int(index))
		return C.GoBytes(pointer, length)
	case sqliteNull:
		return nil
	default:
		return nil
	}
}
