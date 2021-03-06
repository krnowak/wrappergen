package test

import (
	"context"
	"reflect"

	"database/sql/driver"
)

//go:generate wrappergen -basetype=driver.Driver -exttypes=driver.DriverContext -extrafields=extra,interface{} -prefix=realDD -newfuncname=newDriver

//go:generate wrappergen -basetype=driver.Connector -prefix=realDCC -newfuncname=newConnector -extrafields extra,interface{}

//go:generate wrappergen -basetype=driver.Conn -exttypes=driver.ConnBeginTx;driver.ConnPrepareContext;driver.Execer;driver.ExecerContext;driver.NamedValueChecker;driver.Pinger;driver.Queryer;driver.QueryerContext;driver.SessionResetter -extrafields=extra,interface{} -prefix=realDC -newfuncname=newConn

//go:generate wrappergen -basetype=driver.Stmt -exttypes=driver.ColumnConverter;driver.NamedValueChecker;driver.StmtExecContext;driver.StmtQueryContext -extrafields=extra,interface{} -prefix=realDS -newfuncname=newStmt

//go:generate wrappergen -basetype=driver.Rows -exttypes=driver.RowsColumnTypeDatabaseTypeName;driver.RowsColumnTypeLength;driver.RowsColumnTypeNullable;driver.RowsColumnTypePrecisionScale;driver.RowsColumnTypeScanType;driver.RowsNextResultSet -extrafields=extra,interface{} -prefix=realDR -newfuncname=newRows

//go:generate wrappergen -basetype=driver.Tx -prefix=realDT -newfuncname=newTx -extrafields extra,interface{}

// driver.Driver functions for driver.Conn

func realDDOpen(r driver.Driver, extra interface{}, name string) (driver.Conn, error) {
	realConn, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	return newConn(realConn, extra), nil
}

// driver.Driver functions for driver.DriverContext

func realDDOpenConnector(r driver.DriverContext, extra interface{}, name string) (driver.Connector, error) {
	realConnector, err := r.OpenConnector(name)
	if err != nil {
		return nil, err
	}
	return newConnector(realConnector, extra), nil
}

// driver.Conn functions for driver.Conn

func realDCPrepare(r driver.Conn, extra interface{}, query string) (driver.Stmt, error) {
	stmt, err := r.Prepare(query)
	if err != nil {
		return nil, err
	}
	return newStmt(stmt, extra), nil
}

func realDCClose(r driver.Conn, extra interface{}) error {
	return r.Close()
}

func realDCBegin(r driver.Conn, extra interface{}) (driver.Tx, error) {
	realTx, err := r.Begin()
	if err != nil {
		return nil, err
	}
	return newTx(realTx, extra), err
}

// driver.Conn functions for driver.ConnBeginTx

func realDCBeginTx(r driver.ConnBeginTx, extra interface{}, ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	realTx, err := r.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return newTx(realTx, extra), nil
}

// driver.Conn functions for driver.ConnPrepareContext

func realDCPrepareContext(r driver.ConnPrepareContext, extra interface{}, ctx context.Context, query string) (driver.Stmt, error) {
	realStmt, err := r.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return newStmt(realStmt, extra), err
}

// driver.Conn functions for driver.Execer

func realDCExec(r driver.Execer, extra interface{}, query string, args []driver.Value) (driver.Result, error) {
	return r.Exec(query, args)
}

// driver.Conn functions for driver.ExecerContext

func realDCExecContext(r driver.ExecerContext, extra interface{}, ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return r.ExecContext(ctx, query, args)
}

// driver.Conn functions for driver.NamedValueChecker

func realDCCheckNamedValue(r driver.NamedValueChecker, extra interface{}, value *driver.NamedValue) error {
	return r.CheckNamedValue(value)
}

// driver.Conn functions for driver.Pinger

func realDCPing(r driver.Pinger, extra interface{}, ctx context.Context) error {
	return r.Ping(ctx)
}

// driver.Conn functions for driver.Queryer

func realDCQuery(r driver.Queryer, extra interface{}, query string, args []driver.Value) (driver.Rows, error) {
	realRows, err := r.Query(query, args)
	if err != nil {
		return nil, err
	}
	return newRows(realRows, extra), nil
}

// driver.Conn functions for driver.QueryerContext

func realDCQueryContext(r driver.QueryerContext, extra interface{}, ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	realRows, err := r.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return newRows(realRows, extra), nil
}

// driver.Conn functions for driver.SessionResetter

func realDCResetSession(r driver.SessionResetter, extra interface{}, ctx context.Context) error {
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

func realDCCConnect(r driver.Connector, extra interface{}, ctx context.Context) (driver.Conn, error) {
	realConn, err := r.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return newConn(realConn, extra), nil
}

func realDCCDriver(r driver.Connector, extra interface{}) driver.Driver {
	return newDriver(r.Driver(), extra)
}

func realDTCommit(r driver.Tx, extra interface{}) error {
	return r.Commit()
}

func realDTRollback(r driver.Tx, extra interface{}) error {
	return r.Rollback()
}
