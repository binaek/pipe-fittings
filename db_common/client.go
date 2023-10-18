package db_common

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/turbot/pipe-fittings/queryresult"
)

type Client interface {
	Close(context.Context) error
	LoadUserSearchPath(context.Context) error

	// TODO KAI MOVE TO SteampipeClient
	SetRequiredSessionSearchPath(context.Context) error
	GetRequiredSessionSearchPath() []string
	GetCustomSearchPath() []string

	// acquire a management database connection - must be closed
	AcquireManagementConnection(context.Context) (*sql.Conn, error)
	// acquire a query execution session (which search pathand cache options  set) - must be closed
	AcquireSession(context.Context) *AcquireSessionResult

	ExecuteSync(context.Context, string, ...any) (*queryresult.SyncQueryResult, error)
	Execute(context.Context, string, ...any) (*queryresult.Result, error)

	ExecuteSyncInSession(context.Context, *DatabaseSession, string, ...any) (*queryresult.SyncQueryResult, error)
	ExecuteInSession(context.Context, *DatabaseSession, func(), string, ...any) (*queryresult.Result, error)

	ResetPools(context.Context)
	GetSchemaFromDB(context.Context) (*SchemaMetadata, error)

	// TODO KAI REMOVED
	//ServerSettings() *ServerSettings
	RegisterNotificationListener(f func(notification *pgconn.Notification))
	GetConnectionString() string
}
