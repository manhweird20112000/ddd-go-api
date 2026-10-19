package contract_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	grpcadapter "github.com/yourorg/go-template/internal/modules/identity/adapter/grpc"
	"github.com/yourorg/go-template/internal/modules/identity/adapter/grpcclient"
	"github.com/yourorg/go-template/internal/modules/identity/adapter/inproc"
	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
	identityv1 "github.com/yourorg/go-template/internal/platform/grpcx/gen/identity/v1"
)

type query struct{ created time.Time }

func (q query) GetUser(_ context.Context, id string) (domain.User, error) {
	if id == "missing" {
		return domain.User{}, domain.ErrNotFound
	}
	return domain.User{ID: id, Email: "user@example.test", CreatedAt: q.created,
		PasswordHash: "private-hash"}, nil
}

func TestLocalAndGRPCContract(t *testing.T) {
	created := time.Now().UTC().Truncate(time.Second)
	local := inproc.New(query{created: created})
	t.Run("local", func(t *testing.T) { runContractCases(t, local, created) })

	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(server, grpcadapter.New(local))
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	t.Run("grpc", func(t *testing.T) { runContractCases(t, grpcclient.New(conn), created) })
}

func runContractCases(t *testing.T, service contract.Service, created time.Time) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	user, err := service.GetUser(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "user-1" || user.Email != "user@example.test" || !user.CreatedAt.Equal(created) {
		t.Fatalf("unexpected contract user: %+v", user)
	}
	_, err = service.GetUser(ctx, "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
