// Package response defines the single JSON envelope every endpoint replies
// with, so a client can branch on `status`/`code` without knowing which route
// it called or whether the failure came from validation, the store, or a panic.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Status values for the envelope's `status` field.
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// Keys under which middleware and handlers stash per-request values on the Gin
// context. They live here because both the middleware and Error() touch them.
const (
	CtxRequestID = "requestId" // set by middleware.RequestID
	CtxErrorCode = "errorCode" // set by Error, read by the request logger
)

// Envelope is the shape of every response body.
//
//	{"status":"success","code":200,"message":"...","data":{...}}
//	{"status":"error","code":404,"message":"...","error":"not_found","data":null}
//
// Data is always present (null on failure) so clients can read it unguarded.
type Envelope struct {
	Status     string      `json:"status"`
	Code       int         `json:"code"`    // HTTP status code, mirrored into the body
	Message    string      `json:"message"` // human-readable, safe to show to a user
	Data       any         `json:"data"`
	Error      string      `json:"error,omitempty"`      // machine-readable slug, errors only
	RequestID  string      `json:"requestId,omitempty"`  // errors only; matches the log line
	Pagination *Pagination `json:"pagination,omitempty"` // list endpoints only
}

// Pagination is the metadata returned alongside a paginated list.
type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`      // total rows matching the (optional) filter
	TotalPages int `json:"totalPages"` // ceil(total / limit)
}

// Success writes a 2xx envelope carrying data.
func Success(c *gin.Context, status int, message string, data any) {
	c.JSON(status, Envelope{
		Status:  StatusSuccess,
		Code:    status,
		Message: message,
		Data:    data,
	})
}

// OK is Success with 200.
func OK(c *gin.Context, message string, data any) {
	Success(c, http.StatusOK, message, data)
}

// Created is Success with 201.
func Created(c *gin.Context, message string, data any) {
	Success(c, http.StatusCreated, message, data)
}

// List writes a 200 envelope whose data is the page of items, with the
// pagination block alongside it.
func List(c *gin.Context, message string, data any, p Pagination) {
	c.JSON(http.StatusOK, Envelope{
		Status:     StatusSuccess,
		Code:       http.StatusOK,
		Message:    message,
		Data:       data,
		Pagination: &p,
	})
}

// Error writes a failure envelope and aborts the handler chain, so middleware
// downstream never writes a second body. The error slug is stashed on the
// context so the request logger can record which failure the client was given.
func Error(c *gin.Context, status int, code, message string) {
	c.Set(CtxErrorCode, code)
	c.AbortWithStatusJSON(status, Envelope{
		Status:    StatusError,
		Code:      status,
		Message:   message,
		Data:      nil,
		Error:     code,
		RequestID: c.GetString(CtxRequestID),
	})
}
