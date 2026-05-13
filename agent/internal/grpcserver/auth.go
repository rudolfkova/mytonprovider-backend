package grpcserver

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func authInterceptor(expectedToken string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// grpc.health.v1.Health/Check must work without Bearer (Kubernetes grpc_health_probe, etc.).
		if info.FullMethod == grpc_health_v1.Health_Check_FullMethodName {
			return handler(ctx, req)
		}
		if !isAuthorized(ctx, expectedToken) {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization token")
		}
		return handler(ctx, req)
	}
}

func isAuthorized(ctx context.Context, expectedToken string) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}

	authHeaderValues := md.Get("authorization")
	if len(authHeaderValues) == 0 {
		return false
	}

	authHeader := strings.TrimSpace(authHeaderValues[0])
	if authHeader == "" {
		return false
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) == 1
}
