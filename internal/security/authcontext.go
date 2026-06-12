package security

import "context"

type authorizedOperationKey struct{}

// WithAuthorizedOperation stores the authorized operation on ctx for endpoint reconciliation.
// WithAuthorizedOperation 将已授权 operation 存入 ctx，供 endpoint 对齐校验。
func WithAuthorizedOperation(ctx context.Context, operation OperationRequest) context.Context {
	return context.WithValue(ctx, authorizedOperationKey{}, operation)
}

// AuthorizedOperationFromContext returns the authorized operation stored on ctx.
// AuthorizedOperationFromContext 返回 ctx 中存储的已授权 operation。
func AuthorizedOperationFromContext(ctx context.Context) (OperationRequest, bool) {
	if ctx == nil {
		return OperationRequest{}, false
	}
	operation, ok := ctx.Value(authorizedOperationKey{}).(OperationRequest)
	return operation, ok
}
