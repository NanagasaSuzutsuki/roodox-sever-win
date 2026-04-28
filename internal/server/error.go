package server

import (
	"errors"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toGrpcError 将本地错误转成 gRPC 标准错误
func toGrpcError(err error) error {
	if err == nil {
		return nil
	}

	// 文件不存在
	if errors.Is(err, os.ErrNotExist) {
		return status.Error(codes.NotFound, err.Error())
	}

	// 权限不足
	if errors.Is(err, os.ErrPermission) {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	// 已存在
	if errors.Is(err, os.ErrExist) {
		return status.Error(codes.AlreadyExists, err.Error())
	}

	// 文件系统错误
	if errors.Is(err, os.ErrInvalid) {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	// 默认内部错误
	return status.Error(codes.Internal, err.Error())
}

func invalidArgument(message string) error {
	return status.Error(codes.InvalidArgument, message)
}

func failedPrecondition(message string) error {
	return status.Error(codes.FailedPrecondition, message)
}
