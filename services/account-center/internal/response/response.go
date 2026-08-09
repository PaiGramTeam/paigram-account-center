package response

import (
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}

// ErrorDetail represents detailed error information
type ErrorDetail struct {
	Code    string      `json:"code"`              // Error code for client-side handling
	Message string      `json:"message"`           // User-friendly error message
	Details interface{} `json:"details,omitempty"` // Optional additional error details
}

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Total      int64 `json:"total"`       // Total number of records
	Page       int   `json:"page"`        // Current page number
	PageSize   int   `json:"page_size"`   // Number of records per page
	TotalPages int   `json:"total_pages"` // Total number of pages
}

// PaginatedPayload represents the canonical paginated payload shape.
type PaginatedPayload struct {
	Items      interface{}     `json:"items"`          // Paginated items
	Pagination *PaginationMeta `json:"pagination"`     // Pagination metadata
	Meta       interface{}     `json:"meta,omitempty"` // Optional payload-specific metadata
}

// PaginatedResponse is kept as an alias for existing references.
type PaginatedResponse = PaginatedPayload

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Data:    data,
		Message: "success",
	})
}

func SuccessWithMessage(c *gin.Context, data interface{}, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Data:    data,
		Message: message,
	})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    http.StatusCreated,
		Data:    data,
		Message: "created successfully",
	})
}

func Error(c *gin.Context, httpCode int, message string) {
	c.JSON(httpCode, Response{
		Code:    httpCode,
		Data:    nil,
		Message: message,
	})
}

func ErrorWithData(c *gin.Context, httpCode int, message string, data interface{}) {
	c.JSON(httpCode, Response{
		Code:    httpCode,
		Data:    data,
		Message: message,
	})
}

func ErrorWithCode(c *gin.Context, httpCode int, errCode string, message string, details interface{}) {
	errorDetail := ErrorDetail{
		Code:    errCode,
		Message: message,
		Details: details,
	}
	c.JSON(httpCode, gin.H{
		"error": errorDetail,
	})
}

func BadRequestWithCode(c *gin.Context, errCode string, message string, details interface{}) {
	ErrorWithCode(c, http.StatusBadRequest, errCode, message, details)
}

func UnauthorizedWithCode(c *gin.Context, errCode string, message string, details interface{}) {
	ErrorWithCode(c, http.StatusUnauthorized, errCode, message, details)
}

func ForbiddenWithCode(c *gin.Context, errCode string, message string, details interface{}) {
	ErrorWithCode(c, http.StatusForbidden, errCode, message, details)
}

func NotFoundWithCode(c *gin.Context, errCode string, message string, details interface{}) {
	ErrorWithCode(c, http.StatusNotFound, errCode, message, details)
}

func ConflictWithCode(c *gin.Context, errCode string, message string, details interface{}) {
	ErrorWithCode(c, http.StatusConflict, errCode, message, details)
}

func InternalServerErrorWithCode(c *gin.Context, errCode string, message string, details interface{}) {
	ErrorWithCode(c, http.StatusInternalServerError, errCode, message, details)
}

func TooManyRequestsWithCode(c *gin.Context, errCode string, message string, details interface{}) {
	ErrorWithCode(c, http.StatusTooManyRequests, errCode, message, details)
}

func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, message)
}

func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, message)
}

func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, message)
}

func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, message)
}

func Conflict(c *gin.Context, message string) {
	Error(c, http.StatusConflict, message)
}

func InternalServerError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, message)
}

func TooManyRequests(c *gin.Context, message string) {
	Error(c, http.StatusTooManyRequests, message)
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func Custom(c *gin.Context, httpCode int, code int, data interface{}, message string) {
	c.JSON(httpCode, Response{
		Code:    code,
		Data:    data,
		Message: message,
	})
}

func SuccessWithPagination(c *gin.Context, data interface{}, total int64, page, pageSize int) {
	SuccessWithPaginationMeta(c, data, total, page, pageSize, nil)
}

// SuccessWithPaginationMeta returns a paginated success response with optional metadata.
func SuccessWithPaginationMeta(c *gin.Context, items interface{}, total int64, page, pageSize int, meta interface{}) {
	paginatedData := PaginatedPayload{
		Items:      items,
		Pagination: NewPaginationMeta(total, page, pageSize),
		Meta:       meta,
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Data:    paginatedData,
		Message: "success",
	})
}

// NewPaginationMeta creates a new PaginationMeta instance
func NewPaginationMeta(total int64, page, pageSize int) *PaginationMeta {
	if total == 0 || pageSize <= 0 {
		return &PaginationMeta{
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: 0,
		}
	}

	pageSize64 := int64(pageSize)
	totalPages64 := total / pageSize64
	if total%pageSize64 > 0 {
		totalPages64++
	}

	totalPages := math.MaxInt
	if totalPages64 <= int64(math.MaxInt) {
		totalPages = int(totalPages64)
	}

	return &PaginationMeta{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}
