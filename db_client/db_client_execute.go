package db_client

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/turbot/go-kit/helpers"
	"github.com/turbot/pipe-fittings/constants"
	"github.com/turbot/pipe-fittings/db_common"
	"github.com/turbot/pipe-fittings/error_helpers"
	"github.com/turbot/pipe-fittings/queryresult"
	"github.com/turbot/pipe-fittings/statushooks"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// ExecuteSync implements Client
// execute a query against this client and wait for the result
func (c *DbClient) ExecuteSync(ctx context.Context, query string, args ...any) (*queryresult.SyncQueryResult, error) {
	// acquire a session
	sessionResult := c.AcquireSession(ctx)
	if sessionResult.Error != nil {
		return nil, sessionResult.Error
	}

	// set setShouldShowTiming flag
	// (this will refetch ScanMetadataMaxId if timing has just been enabled)
	c.setShouldShowTiming(ctx, sessionResult.Session)

	defer func() {
		// we need to do this in a closure, otherwise the ctx will be evaluated immediately
		// and not in call-time
		sessionResult.Session.Close(error_helpers.IsContextCanceled(ctx))
	}()
	return c.ExecuteSyncInSession(ctx, sessionResult.Session, query, args...)
}

// ExecuteSyncInSession implements Client
// execute a query against this client and wait for the result
func (c *DbClient) ExecuteSyncInSession(ctx context.Context, session *db_common.DatabaseSession, query string, args ...any) (*queryresult.SyncQueryResult, error) {
	if query == "" {
		return &queryresult.SyncQueryResult{}, nil
	}

	result, err := c.ExecuteInSession(ctx, session, nil, query, args...)
	if err != nil {
		return nil, error_helpers.WrapError(err)
	}

	syncResult := &queryresult.SyncQueryResult{Cols: result.Cols}
	for row := range *result.RowChan {
		select {
		case <-ctx.Done():
		default:
			// save the first row error to return
			if row.Error != nil && err == nil {
				err = error_helpers.WrapError(row.Error)
			}
			syncResult.Rows = append(syncResult.Rows, row)
		}
	}
	if c.shouldShowTiming() {
		syncResult.TimingResult = <-result.TimingResult
	}

	return syncResult, err
}

// Execute implements Client
// execute the query in the given Context
// NOTE: The returned Result MUST be fully read - otherwise the connection will block and will prevent further communication
func (c *DbClient) Execute(ctx context.Context, query string, args ...any) (*queryresult.Result, error) {
	// acquire a session
	sessionResult := c.AcquireSession(ctx)
	if sessionResult.Error != nil {
		return nil, sessionResult.Error
	}
	// disable statushooks when timing is enabled, because setShouldShowTiming internally calls the readRows funcs which
	// calls the statushooks.Done, which hides the `Executing query…` spinner, when timing is enabled.
	timingCtx := statushooks.DisableStatusHooks(ctx)

	// re-read ArgTiming from viper (in case the .timing command has been run)
	// (this will refetch ScanMetadataMaxId if timing has just been enabled)
	c.setShouldShowTiming(timingCtx, sessionResult.Session)

	// define callback to close session when the async execution is complete
	closeSessionCallback := func() { sessionResult.Session.Close(error_helpers.IsContextCanceled(ctx)) }
	return c.ExecuteInSession(ctx, sessionResult.Session, closeSessionCallback, query, args...)
}

// ExecuteInSession implements Client
// execute the query in the given Context using the provided DatabaseSession
// ExecuteInSession assumes no responsibility over the lifecycle of the DatabaseSession - that is the responsibility of the caller
// NOTE: The returned Result MUST be fully read - otherwise the connection will block and will prevent further communication
func (c *DbClient) ExecuteInSession(ctx context.Context, session *db_common.DatabaseSession, onComplete func(), query string, args ...any) (res *queryresult.Result, err error) {
	if query == "" {
		return queryresult.NewResult(nil), nil
	}

	// fail-safes
	if session == nil {
		return nil, fmt.Errorf("nil session passed to ExecuteInSession")
	}
	if session.Connection == nil {
		return nil, fmt.Errorf("nil database connection passed to ExecuteInSession")
	}
	startTime := time.Now()
	// get a context with a timeout for the query to execute within
	// we don't use the cancelFn from this timeout context, since usage will lead to 'pgx'
	// prematurely closing the database connection that this query executed in
	ctxExecute := c.getExecuteContext(ctx)

	var tx *sql.Tx

	defer func() {
		if err != nil {
			err = error_helpers.HandleQueryTimeoutError(err)
			// stop spinner in case of error
			statushooks.Done(ctxExecute)
			// error - rollback transaction if we have one
			if tx != nil {
				_ = tx.Rollback()
			}
			// in case of error call the onComplete callback
			if onComplete != nil {
				onComplete()
			}
		}
	}()

	// start query
	var rows *sql.Rows
	rows, err = c.startQueryWithRetries(ctxExecute, session, query, args...)
	if err != nil {
		return
	}

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return
	}
	colDefs := fieldDescriptionsToColumns(colTypes, session.Connection)

	result := queryresult.NewResult(colDefs)

	// read the rows in a go routine
	go func() {
		// define a callback which fetches the timing information
		// this will be invoked after reading rows is complete but BEFORE closing the rows object (which closes the connection)
		timingCallback := func() {
			c.getQueryTiming(ctxExecute, startTime, session, result.TimingResult)
		}

		// read in the rows and stream to the query result object
		c.readRows(ctxExecute, rows, result, timingCallback)

		// call the completion callback - if one was provided
		if onComplete != nil {
			onComplete()
		}
	}()

	return result, nil
}

func (c *DbClient) getExecuteContext(ctx context.Context) context.Context {
	queryTimeout := time.Duration(viper.GetInt(constants.ArgDatabaseQueryTimeout)) * time.Second
	// if timeout is zero, do not set a timeout
	if queryTimeout == 0 {
		return ctx
	}
	// create a context with a deadline
	shouldBeDoneBy := time.Now().Add(queryTimeout)
	//nolint:golint,lostcancel //we don't use this cancel fn because, pgx prematurely cancels the PG connection when this cancel gets called in 'defer'
	newCtx, _ := context.WithDeadline(ctx, shouldBeDoneBy)

	return newCtx
}

func (c *DbClient) getQueryTiming(ctx context.Context, startTime time.Time, session *db_common.DatabaseSession, resultChannel chan *queryresult.TimingResult) {
	if !c.shouldShowTiming() {
		return
	}

	var timingResult = &queryresult.TimingResult{
		Duration: time.Since(startTime),
	}
	// disable fetching timing information to avoid recursion
	c.disableTiming = true

	// whatever happens, we need to reenable timing, and send the result back with at least the duration
	defer func() {
		c.disableTiming = false
		resultChannel <- timingResult
	}()

	var scanRows *ScanMetadataRow
	err := db_common.ExecuteSystemClientCall(ctx, session.Connection, func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("select id, rows_fetched, cache_hit, hydrate_calls from %s.%s where id > %d", constants.InternalSchema, constants.ForeignTableScanMetadata, session.ScanMetadataMaxId)
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		scanRows, err = db_common.CollectOneToStructByName[ScanMetadataRow](rows)
		return err
	})

	// if we failed to read scan metadata (either because the query failed or the plugin does not support it) just return
	// we don't return the error, since we don't want to error out in this case
	if err != nil || scanRows == nil {
		return
	}

	// so we have scan metadata - create the metadata struct
	timingResult.Metadata = &queryresult.TimingMetadata{}
	timingResult.Metadata.HydrateCalls += scanRows.HydrateCalls
	if scanRows.CacheHit {
		timingResult.Metadata.CachedRowsFetched += scanRows.RowsFetched
	} else {
		timingResult.Metadata.RowsFetched += scanRows.RowsFetched
	}
	// update the max id for this session
	session.ScanMetadataMaxId = scanRows.Id
}

func (c *DbClient) updateScanMetadataMaxId(ctx context.Context, session *db_common.DatabaseSession) error {
	return db_common.ExecuteSystemClientCall(ctx, session.Connection, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, fmt.Sprintf("select max(id) from %s.%s", constants.InternalSchema, constants.ForeignTableScanMetadata))
		err := row.Scan(&session.ScanMetadataMaxId)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
}

// run query in a goroutine, so we can check for cancellation
// in case the client becomes unresponsive and does not respect context cancellation
func (c *DbClient) startQuery(ctx context.Context, conn *sql.Conn, query string, args ...any) (rows *sql.Rows, err error) {
	doneChan := make(chan bool)
	go func() {
		// start asynchronous query
		rows, err = conn.QueryContext(ctx, query, args...)
		close(doneChan)
	}()

	select {
	case <-doneChan:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (c *DbClient) readRows(ctx context.Context, rows *sql.Rows, result *queryresult.Result, timingCallback func()) {
	// defer this, so that these get cleaned up even if there is an unforeseen error
	defer func() {
		// we are done fetching results. time for display. clear the status indication
		statushooks.Done(ctx)
		// call the timing callback BEFORE closing the rows
		timingCallback()
		// close the sql rows object
		rows.Close()
		if err := rows.Err(); err != nil {
			result.StreamError(err)
		}
		// close the channels in the result object
		result.Close()

	}()

	rowCount := 0
Loop:
	for rows.Next() {
		select {
		case <-ctx.Done():
			statushooks.SetStatus(ctx, "Cancelling query")
			break Loop
		default:
			rowResult, err := c.readRow(rows, result.Cols)
			if err != nil {
				// the error will be streamed in the defer
				break Loop
			}

			// TACTICAL
			// determine whether to stop the spinner as soon as we stream a row or to wait for completion
			if isStreamingOutput() {
				statushooks.Done(ctx)
			}

			result.StreamRow(rowResult)

			// update the status message with the count of rows that have already been fetched
			// this will not show if the spinner is not active
			statushooks.SetStatus(ctx, fmt.Sprintf("Loading results: %3s", humanizeRowCount(rowCount)))
			rowCount++
		}
	}
}

func (c *DbClient) rowValues(rows *sql.Rows) ([]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	// Create an array of interface{} to store the retrieved values
	values := make([]interface{}, len(columns))
	// create an array to store the pointers to the values
	ptrs := make([]interface{}, len(columns))
	for i := range values {
		ptrs[i] = &values[i]
	}
	// Use a variadic to scan values into the array
	err = rows.Scan(ptrs...)
	if err != nil {
		return nil, err
	}
	return values, rows.Err()
}

func (c *DbClient) readRow(rows *sql.Rows, cols []*queryresult.ColumnDef) ([]any, error) {
	columnValues, err := c.rowValues(rows)
	if err != nil {
		return nil, error_helpers.WrapError(err)
	}
	return c.rowReader.Read(columnValues, cols)
}

func isStreamingOutput() bool {
	outputFormat := viper.GetString(constants.ArgOutput)

	return helpers.StringSliceContains([]string{constants.OutputFormatCSV, constants.OutputFormatLine}, outputFormat)
}

func humanizeRowCount(count int) string {
	p := message.NewPrinter(language.English)
	return p.Sprintf("%d", count)
}
