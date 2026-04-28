package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const requestIDHeader = "x-request-id"

type contextKey string

const (
	requestIDContextKey contextKey = "request_id"
	methodContextKey    contextKey = "grpc_method"
)

type pathGetter interface {
	GetPath() string
}

type buildIDGetter interface {
	GetBuildId() string
}

type unitPathGetter interface {
	GetUnitPath() string
}

type grpcWrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *grpcWrappedServerStream) Context() context.Context {
	return s.ctx
}

func loggingUnaryInterceptor(metrics RuntimeMetrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = injectRequestContext(ctx, info.FullMethod)
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)
		logRPCResult(ctx, info.FullMethod, req, resp, err, duration)
		if metrics != nil {
			metrics.RecordRPC(info.FullMethod, status.Code(err), duration)
		}
		return resp, err
	}
}

func loggingStreamInterceptor(metrics RuntimeMetrics) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := injectRequestContext(ss.Context(), info.FullMethod)
		start := time.Now()
		err := handler(srv, &grpcWrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		})
		duration := time.Since(start)
		logRPCResult(ctx, info.FullMethod, nil, nil, err, duration)
		if metrics != nil {
			metrics.RecordRPC(info.FullMethod, status.Code(err), duration)
		}
		return err
	}
}

func injectRequestContext(ctx context.Context, method string) context.Context {
	requestID := requestIDFromMetadata(ctx)
	if requestID == "" {
		requestID = newRequestID()
	}
	return context.WithValue(
		context.WithValue(ctx, requestIDContextKey, requestID),
		methodContextKey,
		method,
	)
}

func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDContextKey).(string); ok {
		return v
	}
	return ""
}

func MethodFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(methodContextKey).(string); ok {
		return v
	}
	return ""
}

func logRPCResult(ctx context.Context, method string, req any, resp any, err error, duration time.Duration) {
	path := extractPath(req)
	if path == "" {
		path = extractPath(resp)
	}

	buildID := extractBuildID(req)
	if buildID == "" {
		buildID = extractBuildID(resp)
	}

	if buildID == "" {
		buildID = extractUnitPath(req)
	}

	log.Printf(
		"component=grpc request_id=%s method=%s path=%q build_id=%q code=%s duration_ms=%d",
		RequestIDFromContext(ctx),
		method,
		path,
		buildID,
		status.Code(err).String(),
		duration.Milliseconds(),
	)
}

func LogRequestEvent(ctx context.Context, format string, args ...any) {
	prefix := "request_id=" + RequestIDFromContext(ctx)
	if method := MethodFromContext(ctx); method != "" {
		prefix += " method=" + method
	}
	log.Printf(prefix+" "+format, args...)
}

func requestIDFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(requestIDHeader)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}

func extractPath(v any) string {
	if getter, ok := v.(pathGetter); ok {
		return getter.GetPath()
	}
	return ""
}

func extractBuildID(v any) string {
	if getter, ok := v.(buildIDGetter); ok {
		return getter.GetBuildId()
	}
	return ""
}

func extractUnitPath(v any) string {
	if getter, ok := v.(unitPathGetter); ok {
		return getter.GetUnitPath()
	}
	return ""
}

func statusCode(err error) codes.Code {
	return status.Code(err)
}
