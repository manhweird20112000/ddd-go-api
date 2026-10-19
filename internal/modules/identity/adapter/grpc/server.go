// Package grpc exposes the identity contract over generated gRPC protobufs.
package grpcadapter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
	identityv1 "github.com/yourorg/go-template/internal/platform/grpcx/gen/identity/v1"
	"github.com/yourorg/go-template/internal/platform/httpx"
)

// Server adapts identity's public service to protobuf RPCs.
type Server struct{ service contract.Service }

// New creates an IdentityService gRPC implementation.
func New(service contract.Service) *Server { return &Server{service: service} }

// GetUser maps the identity query and its domain errors to gRPC status.
func (s *Server) GetUser(ctx context.Context, req *identityv1.GetUserRequest) (*identityv1.GetUserResponse, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		ids := md.Get("x-request-id")
		if len(ids) > 0 {
			ctx = httpx.WithRequestID(ctx, ids[0])
		}
	}
	user, err := s.service.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, mapError(err)
	}
	return &identityv1.GetUserResponse{
		UserId: user.ID, Email: user.Email, CreatedAt: timestamppb.New(user.CreatedAt),
	}, nil
}

func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, domain.ErrValidation):
		return status.Error(codes.InvalidArgument, "invalid user ID")
	case errors.Is(err, domain.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, "unauthorized")
	case errors.Is(err, domain.ErrConflict):
		return status.Error(codes.AlreadyExists, "conflict")
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
