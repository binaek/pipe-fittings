package pipeline_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/turbot/pipe-fittings/load_mod"
	"github.com/turbot/pipe-fittings/modconfig"
)

// TODO: a comprehensive query trigger test
// 1. Test capture group
// 1. Test pipeline reference
func TestQueryTriggerParse(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	_, triggers, err := load_mod.LoadPipelines(ctx, "./pipelines/query_trigger.fp")
	assert.Nil(err, "error found")

	queryTrigger := triggers["local.trigger.query.query_trigger_no_schedule"]
	if queryTrigger == nil {
		assert.Fail("query_trigger_no_schedule trigger not found")
		return
	}

	st, ok := queryTrigger.Config.(*modconfig.TriggerQuery)
	if !ok {
		assert.Fail("query_trigger_no_schedule trigger is not a query trigger")
		return
	}

	assert.Equal("", st.Schedule)
}
