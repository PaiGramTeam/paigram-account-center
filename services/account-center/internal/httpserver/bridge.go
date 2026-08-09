package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
)

type ginBridgeInput struct{}

type ginBridgeBodyInput struct {
	RawBody []byte `contentType:"*/*"`
}

type ginBridgeOutput struct {
	Status int `status:""`
	Body   []byte
}

type ginContextKey struct{}

type humaContextKey struct{}

func registerGinBridge[I any](api huma.API, op huma.Operation, endpoint gin.HandlerFunc, contract *Contract, requestBody func(*I) []byte) {
	huma.Register[I, ginBridgeOutput](api, op, func(ctx context.Context, input *I) (*ginBridgeOutput, error) {
		ginContext, ok := ctx.Value(ginContextKey{}).(*gin.Context)
		if !ok {
			return nil, errors.New("gin context is unavailable")
		}
		humaContext, ok := ctx.Value(humaContextKey{}).(huma.Context)
		if !ok {
			return nil, errors.New("huma context is unavailable")
		}
		body := requestBody(input)
		if body != nil {
			ginContext.Request.Body = io.NopCloser(bytes.NewReader(body))
			ginContext.Request.ContentLength = int64(len(body))
		}

		originalWriter := ginContext.Writer
		captured := newBufferedGinWriter(originalWriter)
		ginContext.Writer = captured
		func() {
			defer func() { ginContext.Writer = originalWriter }()
			if contract != nil {
				if failure := contract.validateParameters(ginContext); failure != nil {
					ginContext.JSON(failure.status, humaCompatibilityError{
						Code: failure.status, Data: failure.details, Message: failure.message,
					})
					return
				}
				boundBody, failure := contract.bindRequest(ginContext.Request, body)
				if failure != nil {
					ginContext.JSON(failure.status, humaCompatibilityError{
						Code: failure.status, Data: failure.details, Message: failure.message,
					})
					return
				}
				body = boundBody
				ginContext.Request.Body = io.NopCloser(bytes.NewReader(body))
				ginContext.Request.ContentLength = int64(len(body))
			}
			endpoint(ginContext)
		}()
		copyHeaders(humaContext, captured.Header())
		return &ginBridgeOutput{Status: captured.Status(), Body: append([]byte(nil), captured.body.Bytes()...)}, nil
	})
}

type registrationAPI struct {
	huma.API
	adapter  huma.Adapter
	contract *Contract
}

func (a *registrationAPI) Adapter() huma.Adapter {
	return a.adapter
}

func (a *registrationAPI) DocumentOperation(op *huma.Operation) {
	if a.contract != nil {
		a.contract.document(op)
	}
	if !op.Hidden {
		a.OpenAPI().AddOperation(op)
	}
}

type ginRegistrationAdapter struct {
	engine     *gin.Engine
	router     *gin.RouterGroup
	path       string
	middleware []gin.HandlerFunc
}

func (a *ginRegistrationAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	bridge := func(c *gin.Context) {
		originalRequest := c.Request
		humaContext := humagin.NewContext(op, c)
		requestContext := context.WithValue(c.Request.Context(), ginContextKey{}, c)
		requestContext = context.WithValue(requestContext, humaContextKey{}, humaContext)
		c.Request = c.Request.WithContext(requestContext)
		defer func() { c.Request = originalRequest }()
		handler(humaContext)
	}
	handlers := append(append([]gin.HandlerFunc(nil), a.middleware...), bridge)
	a.router.Handle(op.Method, a.path, handlers...)
}

func (a *ginRegistrationAdapter) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	a.engine.ServeHTTP(writer, request)
}

type bufferedGinWriter struct {
	original gin.ResponseWriter
	header   http.Header
	body     bytes.Buffer
	status   int
	size     int
	written  bool
}

func newBufferedGinWriter(original gin.ResponseWriter) *bufferedGinWriter {
	header := make(http.Header, len(original.Header()))
	for name, values := range original.Header() {
		header[name] = append([]string(nil), values...)
	}
	return &bufferedGinWriter{original: original, header: header, status: http.StatusOK, size: -1}
}

func (w *bufferedGinWriter) Header() http.Header {
	return w.header
}

func (w *bufferedGinWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	written, err := w.body.Write(data)
	w.size += written
	return written, err
}

func (w *bufferedGinWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *bufferedGinWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	w.status = status
}

func (w *bufferedGinWriter) WriteHeaderNow() {
	if w.written {
		return
	}
	w.written = true
	w.size = 0
}

func (w *bufferedGinWriter) Status() int {
	return w.status
}

func (w *bufferedGinWriter) Size() int {
	return w.size
}

func (w *bufferedGinWriter) Written() bool {
	return w.written
}

func (w *bufferedGinWriter) Flush() {
	w.WriteHeaderNow()
}

func (w *bufferedGinWriter) CloseNotify() <-chan bool {
	return w.original.CloseNotify()
}

func (w *bufferedGinWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.original.Hijack()
}

func (w *bufferedGinWriter) Pusher() http.Pusher {
	return w.original.Pusher()
}

func copyHeaders(ctx huma.Context, headers http.Header) {
	for name, values := range headers {
		for index, value := range values {
			if index == 0 {
				ctx.SetHeader(name, value)
			} else {
				ctx.AppendHeader(name, value)
			}
		}
	}
}
