package handlers

import (
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// init customises Gin's shared validator. It runs on package import so any
// binary or test that uses these handlers gets the same rules — there is no
// setup call to forget.
func init() {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return // a custom validator is installed; leave it alone
	}

	// Report the JSON name the client actually sent ("stockQuantity"), not the
	// Go field name ("StockQuantity"), in validation messages.
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return f.Name
		}
		return name
	})

	// notblank: a string that is non-empty after trimming whitespace. The stock
	// `required` tag only rejects "", so "   " would pass it.
	_ = v.RegisterValidation("notblank", func(fl validator.FieldLevel) bool {
		return strings.TrimSpace(fl.Field().String()) != ""
	})
}
