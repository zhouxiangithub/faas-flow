package executor

import (
	"fmt"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"io"
	"net/http"
)

type traceHandler struct {
	tracer opentracing.Tracer
	closer io.Closer

	reqSpan    opentracing.Span
	reqSpanCtx opentracing.SpanContext

	nodeSpans      map[string]opentracing.Span
	operationSpans map[string]map[string]opentracing.Span
}

// startReqSpan starts a request span
func (tracerObj *traceHandler) startReqSpan(reqID string) {
	tracerObj.reqSpan = tracerObj.tracer.StartSpan(reqID)
	tracerObj.reqSpan.SetTag("request", reqID)
	tracerObj.reqSpanCtx = tracerObj.reqSpan.Context()
}

// continueReqSpan continue request span
func (tracerObj *traceHandler) continueReqSpan(reqID string, header http.Header) {
	var err error

	tracerObj.reqSpanCtx, err = tracerObj.tracer.Extract(
		opentracing.HTTPHeaders,
		opentracing.HTTPHeadersCarrier(header),
	)
	if err != nil {
		fmt.Printf("[Request %s] failed to continue req span for tracing, error %v\n", reqID, err)
		return
	}

	tracerObj.reqSpan = nil
	// TODO: Its not Supported to get span from spanContext as of now
	//       https://github.com/opentracing/specification/issues/81
	//       it will support us to extend the request span for nodes
	//reqSpan = opentracing.SpanFromContext(reqSpanCtx)
}

// extendReqSpan extend req span over a request
// func extendReqSpan(url string, req *http.Request) {
func (tracerObj *traceHandler) extendReqSpan(reqID string, lastNode string, url string, req *http.Request) {
	// TODO: as requestSpan can't be regenerated with the span context we
	//       forward the nodes SpanContext
	// span := reqSpan
	span := tracerObj.nodeSpans[lastNode]
	if span == nil {
		return
	}

	ext.SpanKindRPCClient.Set(span)
	ext.HTTPUrl.Set(span, url)
	ext.HTTPMethod.Set(span, "POST")
	err := span.Tracer().Inject(
		span.Context(),
		opentracing.HTTPHeaders,
		opentracing.HTTPHeadersCarrier(req.Header),
	)
	if err != nil {
		fmt.Printf("[Request %s] failed to extend req span for tracing, error %v\n", reqID, err)
	}
	if req.Header.Get("Uber-Trace-Id") == "" {
		fmt.Printf("[Request %s] failed to extend req span for tracing, error Uber-Trace-Id not set\n",
			reqID)
	}
}

// stopReqSpan terminate a request span
func (tracerObj *traceHandler) stopReqSpan() {
	if tracerObj.reqSpan == nil {
		return
	}

	tracerObj.reqSpan.Finish()
}

// startNodeSpan starts a node span
func (tracerObj *traceHandler) startNodeSpan(node string, reqID string) {

	tracerObj.nodeSpans[node] = tracerObj.tracer.StartSpan(
		node, ext.RPCServerOption(tracerObj.reqSpanCtx))

	/*
		 tracerObj.nodeSpans[node] = tracerObj.tracer.StartSpan(
			node, opentracing.ChildOf(reqSpan.Context()))
	*/

	tracerObj.nodeSpans[node].SetTag("async", "true")
	tracerObj.nodeSpans[node].SetTag("request", reqID)
	tracerObj.nodeSpans[node].SetTag("node", node)
}

// stopNodeSpan terminates a node span
func (tracerObj *traceHandler) stopNodeSpan(node string) {

	tracerObj.nodeSpans[node].Finish()
}

// startOperationSpan starts an operation span
func (tracerObj *traceHandler) startOperationSpan(node string, reqID string, operationID string) {

	if tracerObj.nodeSpans[node] == nil {
		return
	}

	operationSpans, ok := tracerObj.operationSpans[node]
	if !ok {
		operationSpans = make(map[string]opentracing.Span)
		tracerObj.operationSpans[node] = operationSpans
	}

	nodeContext := tracerObj.nodeSpans[node].Context()
	operationSpans[operationID] = tracerObj.tracer.StartSpan(
		operationID, opentracing.ChildOf(nodeContext))

	operationSpans[operationID].SetTag("request", reqID)
	operationSpans[operationID].SetTag("node", node)
	operationSpans[operationID].SetTag("operation", operationID)
}

// stopOperationSpan stops an operation span
func (tracerObj *traceHandler) stopOperationSpan(node string, operationID string) {

	if tracerObj.nodeSpans[node] == nil {
		return
	}

	operationSpans := tracerObj.operationSpans[node]
	operationSpans[operationID].Finish()
}

// flushTracer flush all pending traces
func (tracerObj *traceHandler) flushTracer() {
	tracerObj.closer.Close()
}
