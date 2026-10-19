package archtest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const modulePath = "github.com/yourorg/go-template"

func TestArchitectureImports(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := packages.Load(&packages.Config{
		Context: context.Background(), Dir: root,
		Mode: packages.NeedName | packages.NeedImports | packages.NeedFiles | packages.NeedModule,
	}, "./internal/...", "./cmd/...")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) == 0 {
		t.Fatal("no packages found for architecture check")
	}
	for _, pkg := range loaded {
		for _, issue := range pkg.Errors {
			t.Errorf("load %s: %s", pkg.PkgPath, issue)
		}
		for imported := range pkg.Imports {
			if err := checkImport(pkg.PkgPath, imported); err != nil {
				t.Error(err)
			}
		}
	}
}

func TestCheckImportRules(t *testing.T) {
	tests := []struct {
		from, to string
		allowed  bool
	}{
		{modulePath + "/internal/modules/notification/adapter/inproc", modulePath + "/internal/modules/identity/contract", true},
		{modulePath + "/internal/modules/notification/app", modulePath + "/internal/modules/identity/domain", false},
		{modulePath + "/internal/modules/identity/domain", "github.com/google/uuid", false},
		{modulePath + "/internal/modules/identity/domain", "net/mail", true},
		{modulePath + "/internal/modules/identity/app", "github.com/jackc/pgx/v5", false},
		{modulePath + "/internal/platform/db", modulePath + "/internal/modules/identity/domain", false},
		{modulePath + "/internal/app", modulePath + "/internal/modules/identity", true},
	}
	for _, tt := range tests {
		err := checkImport(tt.from, tt.to)
		if (err == nil) != tt.allowed {
			t.Errorf("checkImport(%q, %q): got %v, allowed=%v", tt.from, tt.to, err, tt.allowed)
		}
	}
}

func checkImport(from, to string) error {
	modulePrefix := modulePath + "/internal/modules/"
	platformPrefix := modulePath + "/internal/platform/"
	if strings.HasPrefix(from, platformPrefix) && strings.HasPrefix(to, modulePrefix) {
		return fmt.Errorf("platform %s imports module %s", from, to)
	}
	fromModule, fromLayer := moduleAndLayer(from)
	toModule, _ := moduleAndLayer(to)
	if fromModule != "" && toModule != "" && fromModule != toModule {
		if to != modulePrefix+toModule+"/contract" && !strings.HasPrefix(to, modulePrefix+toModule+"/contract/") {
			return fmt.Errorf("module %s imports outside %s contract: %s", fromModule, toModule, to)
		}
	}
	if fromLayer == "domain" {
		first := strings.Split(to, "/")[0]
		if strings.Contains(first, ".") || strings.HasPrefix(to, modulePath) {
			return fmt.Errorf("domain %s imports non-standard package %s", from, to)
		}
	}
	if fromLayer == "app" {
		if strings.HasPrefix(to, platformPrefix) || strings.Contains(to, "/adapter/") ||
			strings.HasPrefix(to, "github.com/jackc/pgx/") || strings.HasPrefix(to, "github.com/go-chi/") ||
			strings.HasPrefix(to, "google.golang.org/grpc") {
			return fmt.Errorf("application %s imports infrastructure %s", from, to)
		}
	}
	return nil
}

func moduleAndLayer(path string) (module, layer string) {
	const prefix = modulePath + "/internal/modules/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	module = parts[0]
	if len(parts) > 1 {
		layer = parts[1]
	}
	return module, layer
}
