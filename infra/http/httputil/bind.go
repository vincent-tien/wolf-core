package httputil

import (
	"github.com/gin-gonic/gin"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
	"github.com/vincent-tien/wolf-core/validator"
)

// BindAndValidate binds the JSON request body to a new instance of T and
// validates it using struct tags. On failure it writes the error response via
// the Default Responder and returns false.
//
//	req, ok := httputil.BindAndValidate[CreateProductRequest](c)
//	if !ok {
//	    return
//	}
func BindAndValidate[T any](c *gin.Context) (T, bool) {
	return BindAndValidateWith[T](Default, c)
}

// BindAndValidateWith is like BindAndValidate but uses the given Responder
// instead of Default.
func BindAndValidateWith[T any](r *Responder, c *gin.Context) (T, bool) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		r.Error(c, sharederrors.NewValidation("", err.Error()))
		return req, false
	}
	if err := validator.Validate(req); err != nil {
		r.Error(c, err)
		return req, false
	}
	return req, true
}
