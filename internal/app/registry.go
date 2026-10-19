package app

import (
	"context"
	"fmt"
	"io"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/yourorg/go-template/internal/modules/identity/adapter/grpcclient"
	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/platform/config"
)

func resolveIdentity(ctx context.Context, cfg config.Config, local contract.Service) (contract.Service, io.Closer, error) {
	if local != nil {
		return local, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	conn, err := grpc.NewClient("dns:///"+cfg.IdentityGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(
			otelgrpc.WithPropagators(propagation.TraceContext{}))))
	if err != nil {
		return nil, nil, fmt.Errorf("create remote identity client: %w", err)
	}
	return grpcclient.New(conn), conn, nil
}
