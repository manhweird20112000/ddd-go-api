// Command sqlc generates database code for every registered module config.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	configs, err := filepath.Glob(filepath.Join("internal", "modules", "*", "sqlc.yaml"))
	if err != nil || len(configs) == 0 {
		fmt.Fprintln(os.Stderr, "find module sqlc configs:", err)
		os.Exit(1)
	}
	for _, config := range configs {
		// #nosec G204 -- config comes only from the fixed internal/modules/*/sqlc.yaml glob.
		command := exec.Command("go", "run", "github.com/sqlc-dev/sqlc/cmd/sqlc@latest", "generate", "-f", config)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "generate", config, ":", err)
			os.Exit(1)
		}
	}
}
