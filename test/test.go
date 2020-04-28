package test

import (
	"context"
	"reflect"

	"database/sql/driver"
)

//go:generate wrappergen -basetype=driver.Conn -exttypes=driver.ConnBeginTx;driver.ConnPrepareContext;driver.Execer;driver.ExecerContext;driver.NamedValueChecker;driver.Pinger;driver.Queryer;driver.QueryerContext;driver.SessionResetter -extrafields=extra,interface{} -prefix=real -newfunc=newConn -imports=context

//go:generate wrappergen -basetype=driver.Stmt -exttypes=driver.ColumnConverter;driver.NamedValueChecker;driver.StmtExecContext;driver.StmtQueryContext -extrafields=extra,interface{} -prefix=realDS -newfunc=newStmt -imports=context

//go:generate wrappergen -basetype=driver.Rows -exttypes=driver.RowsColumnTypeDatabaseTypeName;driver.RowsColumnTypeLength;driver.RowsColumnTypeNullable;driver.RowsColumnTypePrecisionScale;driver.RowsColumnTypeScanType;driver.RowsNextResultSet -extrafields=extra,interface{} -prefix=realDR -newfunc=newRows -imports=reflect

// driver.Conn functions for driver.Conn

func realPrepare(r driver.Conn, extra interface{}, query string) (driver.Stmt, error) {
	stmt, err := r.Prepare(query)
	if err != nil {
		return nil, err
	}
	return newStmt(stmt, extra), nil
}

func realClose(r driver.Conn, extra interface{}) error {
	return r.Close()
}

func realBegin(r driver.Conn, extra interface{}) (driver.Tx, error) {
	realTx, err := r.Begin()
	if err != nil {
		return nil, err
	}
	return newTx(realTx, extra), err
}

// driver.Conn functions for driver.ConnBeginTx

func realBeginTx(r driver.ConnBeginTx, extra interface{}, ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	realTx, err := r.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return newTx(realTx, extra), nil
}

// driver.Conn functions for driver.ConnPrepareContext

func realPrepareContext(r driver.ConnPrepareContext, extra interface{}, ctx context.Context, query string) (driver.Stmt, error) {
	realStmt, err := r.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return newStmt(realStmt, extra), err
}

// driver.Conn functions for driver.Execer

func realExec(r driver.Execer, extra interface{}, query string, args []driver.Value) (driver.Result, error) {
	return r.Exec(query, args)
}

// driver.Conn functions for driver.ExecerContext

func realExecContext(r driver.ExecerContext, extra interface{}, ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return r.ExecContext(ctx, query, args)
}

// driver.Conn functions for driver.NamedValueChecker

func realCheckNamedValue(r driver.NamedValueChecker, extra interface{}, value *driver.NamedValue) error {
	return r.CheckNamedValue(value)
}

// driver.Conn functions for driver.Pinger

func realPing(r driver.Pinger, extra interface{}, ctx context.Context) error {
	return r.Ping(ctx)
}

// driver.Conn functions for driver.Queryer

func realQuery(r driver.Queryer, extra interface{}, query string, args []driver.Value) (driver.Rows, error) {
	realRows, err := r.Query(query, args)
	if err != nil {
		return nil, err
	}
	return newRows(realRows, extra), nil
}

// driver.Conn functions for driver.QueryerContext

func realQueryContext(r driver.QueryerContext, extra interface{}, ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	realRows, err := r.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return newRows(realRows, extra), nil
}

// driver.Conn functions for driver.SessionResetter

func realResetSession(r driver.SessionResetter, extra interface{}, ctx context.Context) error {
	return r.ResetSession(ctx)
}

// driver.Stmt functions for driver.Stmt

func realDSClose(r driver.Stmt, extra interface{}) error {
	return r.Close()
}

func realDSNumInput(r driver.Stmt, extra interface{}) int {
	return r.NumInput()
}

func realDSExec(r driver.Stmt, extra interface{}, args []driver.Value) (driver.Result, error) {
	return r.Exec(args)
}

func realDSQuery(r driver.Stmt, extra interface{}, args []driver.Value) (driver.Rows, error) {
	realRows, err := r.Query(args)
	if err != nil {
		return nil, err
	}
	return newRows(realRows, extra), nil
}

// driver.Stmt functions for driver.ColumnConverter

func realDSColumnConverter(r driver.ColumnConverter, extra interface{}, idx int) driver.ValueConverter {
	return r.ColumnConverter(idx)
}

// driver.Stmt functions for driver.NamedValueChecker

func realDSCheckNamedValue(r driver.NamedValueChecker, extra interface{}, value *driver.NamedValue) error {
	return r.CheckNamedValue(value)
}

// driver.Stmt functions for driver.StmtExecContext

func realDSExecContext(r driver.StmtExecContext, extra interface{}, ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return r.ExecContext(ctx, args)
}

// driver.Stmt functions for driver.StmtQueryContext

func realDSQueryContext(r driver.StmtQueryContext, extra interface{}, ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	realRows, err := r.QueryContext(ctx, args)
	if err != nil {
		return nil, err
	}
	return newRows(realRows, extra), nil
}

// driver.Rows functions for driver.Rows

func realDRColumns(r driver.Rows, extra interface{}) []string {
	return r.Columns()
}

func realDRClose(r driver.Rows, extra interface{}) error {
	return r.Close()
}

func realDRNext(r driver.Rows, extra interface{}, dest []driver.Value) error {
	return r.Next(dest)
}

// driver.Rows functions for driver.RowsColumnTypeDatabaseTypeName

func realDRColumnTypeDatabaseTypeName(r driver.RowsColumnTypeDatabaseTypeName, extra interface{}, index int) string {
	return r.ColumnTypeDatabaseTypeName(index)
}

// driver.Rows functions for driver.RowsColumnTypeLength

func realDRColumnTypeLength(r driver.RowsColumnTypeLength, extra interface{}, index int) (length int64, ok bool) {
	return r.ColumnTypeLength(index)
}

// driver.Rows functions for driver.RowsColumnTypeNullable

func realDRColumnTypeNullable(r driver.RowsColumnTypeNullable, extra interface{}, index int) (nullable, ok bool) {
	return r.ColumnTypeNullable(index)
}

// driver.Rows functions for driver.RowsColumnTypePrecisionScale

func realDRColumnTypePrecisionScale(r driver.RowsColumnTypePrecisionScale, extra interface{}, index int) (precision, scale int64, ok bool) {
	return r.ColumnTypePrecisionScale(index)
}

// driver.Rows functions for driver.RowsColumnTypeScanType

func realDRColumnTypeScanType(r driver.RowsColumnTypeScanType, extra interface{}, index int) reflect.Type {
	return r.ColumnTypeScanType(index)
}

// driver.Rows functions for driver.RowsNextResultSet

func realDRHasNextResultSet(r driver.RowsNextResultSet, extra interface{}) bool {
	return r.HasNextResultSet()
}

func realDRNextResultSet(r driver.RowsNextResultSet, extra interface{}) error {
	return r.NextResultSet()
}

// hand-written stuff for Tx

type tx struct {
	r     driver.Tx
	extra interface{}
}

var _ driver.Tx = &tx{}

func newTx(realTx driver.Tx, extra interface{}) driver.Tx {
	return &tx{
		r:     realTx,
		extra: extra,
	}
}

func (t *tx) Commit() error {
	return t.r.Commit()
}

func (t *tx) Rollback() error {
	return t.r.Rollback()
}
