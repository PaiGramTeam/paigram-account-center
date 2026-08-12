package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

const (
	defaultMaxBodyBytes  = int64(8 << 20)
	defaultBodyReadLimit = 15 * time.Second
)

type BodyOptions struct {
	MaxBytes    int64
	ReadTimeout time.Duration
}

type Contract struct {
	requestType          reflect.Type
	responseType         reflect.Type
	responseAlternatives []reflect.Type
	requestSchema        *huma.Schema
	responseSchema       *huma.Schema
	registry             huma.Registry
	contentType          string
	requestRequired      bool
	successStatus        int
	errors               []int
	errorTypes           map[int]reflect.Type
	parameters           []Parameter
	securitySchemes      []string
	validateBody         bool
	handlerManagedBody   bool
	maxBodyBytes         int64
	bodyReadTimeout      time.Duration
}

// WithSecuritySchemes documents alternative OpenAPI security schemes for a public route.
func (c Contract) WithSecuritySchemes(schemes ...string) Contract {
	c.securitySchemes = append([]string(nil), schemes...)
	return c
}

func (c Contract) WithSuccessResponseAlternatives(responses ...any) Contract {
	for _, response := range responses {
		responseType := indirectType(response)
		if responseType == nil {
			panic("HTTP contract success response type is required")
		}
		c.responseAlternatives = append(c.responseAlternatives, responseType)
	}
	return c
}

func (c Contract) WithOptionalBody() Contract {
	c.requestRequired = false
	return c
}

func (c Contract) WithHandlerManagedBody() Contract {
	c.validateBody = false
	c.handlerManagedBody = true
	return c
}

func (c Contract) WithErrorStatuses(statuses ...int) Contract {
	for _, status := range statuses {
		c.errors = appendStatus(c.errors, status)
	}
	return c
}

func JSONContract(request, response any, successStatus int, errorStatuses ...int) Contract {
	return newContract(request, response, "application/json", successStatus, errorStatuses)
}

func FormContract(request, response any, successStatus int, errorStatuses ...int) Contract {
	return newContract(request, response, "application/x-www-form-urlencoded", successStatus, errorStatuses)
}

func ResponseContract(response any, successStatus int, errorStatuses ...int) Contract {
	return newContract(nil, response, "", successStatus, errorStatuses)
}

func newContract(request, response any, contentType string, successStatus int, errorStatuses []int) Contract {
	if successStatus < 200 || successStatus > 399 {
		panic("HTTP contract success status must be between 200 and 399")
	}
	return Contract{
		requestType:     indirectType(request),
		responseType:    indirectType(response),
		contentType:     contentType,
		requestRequired: request != nil,
		successStatus:   successStatus,
		errors:          append([]int(nil), errorStatuses...),
		errorTypes:      make(map[int]reflect.Type),
		validateBody:    true,
	}
}

func (c Contract) WithErrorResponse(response any, statuses ...int) Contract {
	responseType := indirectType(response)
	if responseType == nil {
		panic("HTTP contract error response type is required")
	}
	if c.errorTypes == nil {
		c.errorTypes = make(map[int]reflect.Type)
	} else {
		cloned := make(map[int]reflect.Type, len(c.errorTypes)+len(statuses))
		for status, current := range c.errorTypes {
			cloned[status] = current
		}
		c.errorTypes = cloned
	}
	for _, status := range statuses {
		c.errors = appendStatus(c.errors, status)
		c.errorTypes[status] = responseType
	}
	return c
}

func (c Contract) WithBodyLimits(maxBytes int64, readTimeout time.Duration) Contract {
	c.maxBodyBytes = maxBytes
	c.bodyReadTimeout = readTimeout
	return c
}

func indirectType(value any) reflect.Type {
	if value == nil {
		return nil
	}
	typeOf := reflect.TypeOf(value)
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	return typeOf
}

func (c Contract) prepare(registry huma.Registry, defaults BodyOptions, path string) Contract {
	c.registry = registry
	c = c.prepareParameters(path)
	if len(c.parameters) > 0 {
		c.errors = appendStatus(c.errors, http.StatusBadRequest)
	}
	if c.requestType != nil {
		c.requestSchema = huma.SchemaFromType(registry, c.requestType)
	}
	if c.responseType != nil {
		c.responseSchema = huma.SchemaFromType(registry, c.responseType)
	}
	if len(c.responseAlternatives) > 0 {
		schemas := make([]*huma.Schema, 0, len(c.responseAlternatives)+1)
		if c.responseSchema != nil {
			schemas = append(schemas, c.responseSchema)
		}
		for _, responseType := range c.responseAlternatives {
			schemas = append(schemas, huma.SchemaFromType(registry, responseType))
		}
		c.responseSchema = &huma.Schema{OneOf: schemas}
	}
	if c.maxBodyBytes == 0 {
		c.maxBodyBytes = defaults.MaxBytes
	}
	if c.bodyReadTimeout == 0 {
		c.bodyReadTimeout = defaults.ReadTimeout
	}
	if c.readsRequestBody() {
		if c.validateBody {
			c.errors = appendStatus(c.errors, http.StatusBadRequest)
			c.errors = appendStatus(c.errors, http.StatusUnsupportedMediaType)
		}
		if c.maxBodyBytes > 0 {
			c.errors = appendStatus(c.errors, http.StatusRequestEntityTooLarge)
		}
		if c.bodyReadTimeout > 0 {
			c.errors = appendStatus(c.errors, http.StatusRequestTimeout)
		}
	}
	return c
}

func appendStatus(statuses []int, status int) []int {
	for _, current := range statuses {
		if current == status {
			return statuses
		}
	}
	return append(statuses, status)
}

func (c Contract) hasRequestBody() bool {
	return c.requestType != nil
}

func (c Contract) readsRequestBody() bool {
	return c.hasRequestBody() && !c.handlerManagedBody
}

func (c Contract) apply(op *huma.Operation) {
	op.DefaultStatus = c.successStatus
	op.Errors = append([]int(nil), c.errors...)
	op.MaxBodyBytes = c.maxBodyBytes
	op.BodyReadTimeout = c.bodyReadTimeout
	op.SkipValidateParams = false
	if len(c.securitySchemes) > 0 {
		op.Security = make([]map[string][]string, 0, len(c.securitySchemes))
		for _, scheme := range c.securitySchemes {
			op.Security = append(op.Security, map[string][]string{scheme: {}})
		}
	}
}

func (c Contract) document(op *huma.Operation) {
	c.documentParameters(op)
	if c.requestSchema == nil {
		op.RequestBody = nil
	} else {
		op.RequestBody = &huma.RequestBody{
			Required: c.requestRequired,
			Content: map[string]*huma.MediaType{
				c.contentType: {Schema: c.requestSchema},
			},
		}
	}
	op.Responses = map[string]*huma.Response{
		strconv.Itoa(c.successStatus): contractResponse(c.successStatus, c.responseSchema),
	}
	for _, status := range c.errors {
		op.Responses[strconv.Itoa(status)] = errorContractResponse(status, c.registry, c.errorTypes[status])
	}
}

func contractResponse(status int, schema *huma.Schema) *huma.Response {
	result := &huma.Response{Description: http.StatusText(status)}
	if status != http.StatusNoContent && schema != nil {
		result.Content = map[string]*huma.MediaType{"application/json": {Schema: schema}}
	}
	return result
}

func errorContractResponse(status int, registry huma.Registry, responseType reflect.Type) *huma.Response {
	var schema *huma.Schema
	if responseType != nil {
		schema = huma.SchemaFromType(registry, responseType)
	} else {
		schema = &huma.Schema{OneOf: []*huma.Schema{
			huma.SchemaFromType(registry, reflect.TypeFor[humaCompatibilityError]()),
			huma.SchemaFromType(registry, reflect.TypeFor[codedCompatibilityError]()),
		}}
	}
	return &huma.Response{
		Description: http.StatusText(status),
		Content: map[string]*huma.MediaType{
			"application/json": {Schema: schema},
		},
	}
}

type routeContracts struct {
	mu     sync.RWMutex
	routes map[string]Contract
}

func newRouteContracts() *routeContracts {
	return &routeContracts{routes: make(map[string]Contract)}
}

func (r *routeContracts) add(method, path string, contract Contract) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := routeKey(method, path)
	if _, exists := r.routes[key]; exists {
		panic("duplicate HTTP contract: " + key)
	}
	r.routes[key] = contract
}

func (r *routeContracts) get(method, path string) (Contract, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	contract, exists := r.routes[routeKey(method, path)]
	return contract, exists
}

func routeKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

type contractValidationFailure struct {
	status  int
	message string
	details []string
}

func (c Contract) bindRequest(request *http.Request, body []byte) ([]byte, *contractValidationFailure) {
	if !c.hasRequestBody() || !c.validateBody {
		return body, nil
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		if c.requestRequired {
			return nil, &contractValidationFailure{status: http.StatusBadRequest, message: "request body is required"}
		}
		return body, nil
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, c.contentType) {
		return nil, &contractValidationFailure{status: http.StatusUnsupportedMediaType, message: "unsupported request content type"}
	}

	var value any
	normalized := body
	switch c.contentType {
	case "application/json":
		if err := json.Unmarshal(body, &value); err != nil {
			return nil, &contractValidationFailure{status: http.StatusBadRequest, message: "invalid JSON request body"}
		}
	case "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, &contractValidationFailure{status: http.StatusBadRequest, message: "invalid form request body"}
		}
		value = firstFormValues(values)
	default:
		return nil, &contractValidationFailure{status: http.StatusUnsupportedMediaType, message: "unsupported request content type"}
	}

	result := &huma.ValidateResult{}
	path := huma.NewPathBuffer([]byte{}, 0)
	path.Push("body")
	huma.Validate(c.registry, c.requestSchema, path, huma.ModeWriteToServer, value, result)
	if len(result.Errors) == 0 {
		if c.contentType == "application/json" {
			typed := reflect.New(c.requestType)
			if err := json.Unmarshal(body, typed.Interface()); err != nil {
				return nil, &contractValidationFailure{status: http.StatusBadRequest, message: "invalid JSON request body"}
			}
			bound, err := json.Marshal(typed.Elem().Interface())
			if err != nil {
				return nil, &contractValidationFailure{status: http.StatusBadRequest, message: "invalid JSON request body"}
			}
			normalized = bound
		}
		return normalized, nil
	}
	details := make([]string, 0, len(result.Errors))
	for _, validationErr := range result.Errors {
		details = append(details, validationErr.Error())
	}
	return nil, &contractValidationFailure{status: http.StatusBadRequest, message: "request body validation failed", details: details}
}

func firstFormValues(values url.Values) map[string]any {
	result := make(map[string]any, len(values))
	for name, entries := range values {
		if len(entries) > 0 {
			result[name] = entries[0]
		}
	}
	return result
}

type humaCompatibilityError struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

type codedCompatibilityError struct {
	Error codedCompatibilityErrorDetail `json:"error"`
}

type codedCompatibilityErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *humaCompatibilityError) Error() string {
	return e.Message
}

func (e *humaCompatibilityError) GetStatus() int {
	return e.Code
}

func installHumaErrorFactory() {
	huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
		details := make([]string, 0, len(errs))
		for _, err := range errs {
			if err != nil {
				details = append(details, err.Error())
			}
		}
		var data any
		if len(details) > 0 {
			data = details
		}
		return &humaCompatibilityError{Code: status, Data: data, Message: message}
	}
	huma.NewErrorWithContext = func(_ huma.Context, status int, message string, errs ...error) huma.StatusError {
		return huma.NewError(status, message, errs...)
	}
}

func validateBodyOptions(options BodyOptions) (BodyOptions, error) {
	if options.MaxBytes == 0 {
		options.MaxBytes = defaultMaxBodyBytes
	}
	if options.ReadTimeout == 0 {
		options.ReadTimeout = defaultBodyReadLimit
	}
	if options.MaxBytes < -1 {
		return BodyOptions{}, errors.New("maximum request body bytes must be positive or -1")
	}
	if options.ReadTimeout < -1 {
		return BodyOptions{}, fmt.Errorf("request body read timeout must be positive or -1: %s", options.ReadTimeout)
	}
	return options, nil
}
