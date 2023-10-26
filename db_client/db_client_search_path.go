package db_client

// TODO THINK ABOUT SEARCH PATHC
//
//// if either a search-path or search-path-prefix is set in config, set the search path
//// (otherwise fall back to user search path)
//// this just sets the required search path for this client
//// - when creating a database session, we will actually set the searchPath
//func (c *DbClient) SetRequiredSessionSearchPath(ctx context.Context) error {
//	configuredSearchPath := viper.GetStringSlice(constants.ArgSearchPath)
//	searchPathPrefix := viper.GetStringSlice(constants.ArgSearchPathPrefix)
//
//	// strip empty elements from search path and prefix
//	configuredSearchPath = helpers.RemoveFromStringSlice(configuredSearchPath, "")
//	searchPathPrefix = helpers.RemoveFromStringSlice(searchPathPrefix, "")
//
//	// default required path to user search path
//	requiredSearchPath := c.userSearchPath
//
//	// store custom search path and search path prefix
//	c.searchPathPrefix = searchPathPrefix
//
//	// if a search path was passed, use that
//	if len(configuredSearchPath) > 0 {
//		requiredSearchPath = configuredSearchPath
//	}
//
//	// add in the prefix if present
//	requiredSearchPath = db_common.AddSearchPathPrefix(searchPathPrefix, requiredSearchPath)
//
//	requiredSearchPath = db_common.EnsureInternalSchemaSuffix(requiredSearchPath)
//
//	// if either configuredSearchPath or searchPathPrefix are set, store requiredSearchPath as customSearchPath
//	if len(configuredSearchPath)+len(searchPathPrefix) > 0 {
//		c.customSearchPath = requiredSearchPath
//	} else {
//		// otherwise clear it
//		c.customSearchPath = nil
//	}
//
//	return nil
//}
//
//func (c *DbClient) LoadUserSearchPath(ctx context.Context) error {
//	conn, err := c.managementPool.Conn(ctx)
//	if err != nil {
//		return err
//	}
//	defer conn.Close()
//	return c.loadUserSearchPath(ctx, conn)
//}
//
//func (c *DbClient) loadUserSearchPath(ctx context.Context, connection *sql.Conn) error {
//	// load the user search path
//	userSearchPath, err := db_common.GetUserSearchPath(ctx, connection)
//	if err != nil {
//		return err
//	}
//	// update the cached value
//	c.userSearchPath = userSearchPath
//	return nil
//}
//
//// GetRequiredSessionSearchPath implements Client
//func (c *DbClient) GetRequiredSessionSearchPath() []string {
//	if c.customSearchPath != nil {
//		return c.customSearchPath
//	}
//
//	return c.userSearchPath
//}
//
//func (c *DbClient) GetCustomSearchPath() []string {
//	return c.customSearchPath
//}
//
//// ensure the search path for the database session is as required
//func (c *DbClient) ensureSessionSearchPath(ctx context.Context, session *db_common.DatabaseSession) error {
//	log.Printf("[TRACE] ensureSessionSearchPath")
//
//	// update the stored value of user search path
//	// this might have changed if a connection has been added/removed
//	if err := c.loadUserSearchPath(ctx, session.Connection); err != nil {
//		return err
//	}
//
//	// get the required search path which is either a custom search path (if present) or the user search path
//	requiredSearchPath := c.GetRequiredSessionSearchPath()
//
//	// now determine whether the session search path is the same as the required search path
//	// if so, return
//	if strings.Join(session.SearchPath, ",") == strings.Join(requiredSearchPath, ",") {
//		log.Printf("[TRACE] session search path is already correct - nothing to do")
//		return nil
//	}
//
//	// so we need to set the search path
//	log.Printf("[TRACE] session search path will be updated to  %s", strings.Join(c.customSearchPath, ","))
//
//	err := db_common.ExecuteSystemClientCall(ctx, session.Connection, func(ctx context.Context, tx *sql.Tx) error {
//		_, err := tx.ExecContext(ctx, fmt.Sprintf("set search_path to %s", strings.Join(db_common.PgEscapeSearchPath(requiredSearchPath), ",")))
//		return err
//	})
//
//	if err == nil {
//		// update the session search path property
//		session.SearchPath = requiredSearchPath
//	}
//	return err
//}
