package db_client

import (
	"context"
	"database/sql"
	"time"

	"github.com/turbot/pipe-fittings/db_common"

	"github.com/spf13/viper"
	"github.com/turbot/pipe-fittings/constants"
	"github.com/turbot/pipe-fittings/utils"
)

const (
	MaxConnLifeTime = 10 * time.Minute
	MaxConnIdleTime = 1 * time.Minute
)

func getDriverNameFromConnectionString(connStr string) string {
	if isPostgresConnectionString(connStr) {
		return "pgx"
	} else if isSqliteConnectionString(connStr) {
		return "sqlite3"
	} else {
		return ""
	}
}

type DbConnectionCallback func(context.Context, *sql.Conn) error

func (c *DbClient) establishConnectionPool(ctx context.Context, overrides clientConfig) error {
	utils.LogTime("db_client.establishConnectionPool start")
	defer utils.LogTime("db_client.establishConnectionPool end")

	pool, err := establishConnectionPool(ctx, c.connectionString)
	if err != nil {
		return err
	}

	// TODO - how do we apply the AfterConnect hook here?
	// the after connect hook used to create and populate the introspection tables

	// apply any overrides
	// this is used to set the pool size and lifetimes of the connections from up top
	overrides.userPoolSettings.apply(pool)

	err = db_common.WaitForPool(
		ctx,
		pool,
		db_common.WithRetryInterval(constants.DBConnectionRetryBackoff),
		db_common.WithTimeout(time.Duration(viper.GetInt(constants.ArgDatabaseStartTimeout))*time.Second),
	)
	if err != nil {
		return err
	}
	c.userPool = pool

	return c.establishManagementConnectionPool(ctx, overrides)
}

// establishSystemConnectionPool creates a connection pool to use to execute
// system-initiated queries (loading of connection state etc.)
// unlike establishConnectionPool, which is run first to create the user-query pool
// this doesn't wait for the pool to completely start, as establishConnectionPool will have established and verified a connection with the service
func (c *DbClient) establishManagementConnectionPool(ctx context.Context, overrides clientConfig) error {
	utils.LogTime("db_client.establishManagementConnectionPool start")
	defer utils.LogTime("db_client.establishManagementConnectionPool end")

	pool, err := establishConnectionPool(ctx, c.connectionString)
	if err != nil {
		return err
	}

	// apply any overrides
	// this is used to set the pool size and lifetimes of the connections from up top
	overrides.managementPoolSettings.apply(pool)

	err = db_common.WaitForPool(
		ctx,
		pool,
		db_common.WithRetryInterval(constants.DBConnectionRetryBackoff),
		db_common.WithTimeout(time.Duration(viper.GetInt(constants.ArgDatabaseStartTimeout))*time.Second),
	)
	if err != nil {
		return err
	}
	c.managementPool = pool

	return nil
}

func establishConnectionPool(ctx context.Context, connectionString string) (*sql.DB, error) {
	driverName := getDriverNameFromConnectionString(connectionString)
	connectionString = getUseableConnectionString(driverName, connectionString)

	pool, err := sql.Open(driverName, connectionString)
	if err != nil {
		return nil, err
	}
	pool.SetConnMaxIdleTime(MaxConnIdleTime)
	pool.SetConnMaxLifetime(MaxConnLifeTime)
	pool.SetMaxOpenConns(db_common.MaxDbConnections())
	return pool, nil
}
