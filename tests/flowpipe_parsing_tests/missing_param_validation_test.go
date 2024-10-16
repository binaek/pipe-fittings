package pipeline_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/turbot/pipe-fittings/load_mod"
	"github.com/turbot/pipe-fittings/parse"
)

func TestMissingParamValidation(t *testing.T) {
	assert := assert.New(t)

	pipelines, _, err := load_mod.LoadPipelines(context.TODO(), "./pipelines/missing_param_validation.fp")
	assert.Nil(err, "error found")

	validateMyParam := pipelines["local.pipeline.missing_param_validation_test"]
	if validateMyParam == nil {
		assert.Fail("missing_param_validation_test pipeline not found")
		return
	}

	stringValid := map[string]interface{}{
		"address_line_2": "Westminster",
	}

	assert.Equal(0, len(parse.ValidateParams(validateMyParam, stringValid, nil)))

	stringInvalid := map[string]interface{}{
		"address_line_2": 123,
	}

	errs := parse.ValidateParams(validateMyParam, stringInvalid, nil)
	assert.Equal(2, len(errs))
	assert.Equal("Bad Request: invalid data type for parameter 'address_line_2' wanted string but received int", errs[0].Error())
	assert.Equal("Bad Request: missing parameter: address_line_2", errs[1].Error())

	invalidParam := map[string]interface{}{
		"invalid": "foo",
	}
	errs = parse.ValidateParams(validateMyParam, invalidParam, nil)
	assert.Equal(2, len(errs))
	assert.Equal("Bad Request: unknown parameter specified 'invalid'", errs[0].Error())
	assert.Equal("Bad Request: missing parameter: address_line_2", errs[1].Error())

	noParam := map[string]interface{}{}
	errs = parse.ValidateParams(validateMyParam, noParam, nil)
	assert.Equal(1, len(errs))
	assert.Equal("Bad Request: missing parameter: address_line_2", errs[0].Error())
}
