// Package grpcclient implements identity's public contract over gRPC.
package grpcclient

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
	identityv1 "github.com/yourorg/go-template/internal/platform/grpcx/gen/identity/v1"
	"github.com/yourorg/go-template/internal/platform/httpx"
)

// Client adapts an identity gRPC connection to the public module contract.
type Client struct {
	rpc     identityv1.IdentityServiceClient
	breaker *gobreaker.CircuitBreaker
}

// New wraps an already configured gRPC connection.
func New(conn grpc.ClientConnInterface) *Client {
	return &Client{
		rpc: identityv1.NewIdentityServiceClient(conn),
		breaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "identity.GetUser", Interval: 30 * time.Second, Timeout: 10 * time.Second,
		}),
	}
}

// GetUser retries this idempotent query with a bounded deadline and jitter.
func (c *Client) GetUser(ctx context.Context, userID string) (contract.User, error) {
	if userID == "" {
		return contract.User{}, fmt.Errorf("identity GetUser: %w", domain.ErrValidation)
	}
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if requestID := httpx.RequestID(ctx); requestID != "" {
		callCtx = metadata.AppendToOutgoingContext(callCtx, "x-request-id", requestID)
	}
	var lastErr error
	for attempt := range 3 {
		value, err := c.breaker.Execute(func() (any, error) {
			return c.rpc.GetUser(callCtx, &identityv1.GetUserRequest{UserId: userID})
		})
		if err == nil {
			response, ok := value.(*identityv1.GetUserResponse)
			if !ok {
				return contract.User{}, fmt.Errorf("identity GetUser: unexpected RPC response type")
			}
			if response.GetCreatedAt() == nil || !response.GetCreatedAt().IsValid() {
				return contract.User{}, fmt.Errorf("identity GetUser: invalid creation time")
			}
			return contract.User{ID: response.GetUserId(), Email: response.GetEmail(),
				CreatedAt: response.GetCreatedAt().AsTime()}, nil
		}
		lastErr = err
		code := status.Code(err)
		if code != codes.Unavailable && code != codes.DeadlineExceeded {
			break
		}
		if attempt == 2 {
			break
		}
		jitter, err := rand.Int(rand.Reader, big.NewInt(30))
		if err != nil {
			return contract.User{}, fmt.Errorf("identity retry jitter: %w", err)
		}
		delay := time.Duration(50*(1<<attempt)+int(jitter.Int64())) * time.Millisecond
		select {
		case <-callCtx.Done():
			return contract.User{}, fmt.Errorf("identity GetUser timeout: %w", callCtx.Err())
		case <-time.After(delay):
		}
	}
	return contract.User{}, mapError(lastErr)
}

func mapError(err error) error {
	switch status.Code(err) {
	case codes.NotFound:
		return fmt.Errorf("identity GetUser: %w", domain.ErrNotFound)
	case codes.InvalidArgument:
		return fmt.Errorf("identity GetUser: %w", domain.ErrValidation)
	case codes.Unauthenticated:
		return fmt.Errorf("identity GetUser: %w", domain.ErrUnauthorized)
	case codes.AlreadyExists:
		return fmt.Errorf("identity GetUser: %w", domain.ErrConflict)
	default:
		return fmt.Errorf("identity GetUser RPC: %w", err)
	}
}
