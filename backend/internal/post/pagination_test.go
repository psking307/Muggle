package post

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

func TestListQueryValidation(t *testing.T) {
	validate := validator.New()

	assert.NoError(t, validate.Struct(ListQuery{Page: 1, PageSize: 10}))
	assert.Error(t, validate.Struct(ListQuery{Page: 0, PageSize: 10}))
	assert.Error(t, validate.Struct(ListQuery{Page: 1, PageSize: 101}))
}
