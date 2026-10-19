// Command coverage checks every module domain and application package against an 80% statement gate.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
)

type count struct{ covered, total int }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	list := exec.Command("go", "list", "./internal/modules/...")
	output, err := list.Output()
	if err != nil {
		return fmt.Errorf("list module packages: %w", err)
	}
	var packages []string
	for _, pkg := range strings.Fields(string(output)) {
		if strings.HasSuffix(pkg, "/domain") || strings.HasSuffix(pkg, "/app") {
			packages = append(packages, pkg)
		}
	}
	if len(packages) == 0 {
		return fmt.Errorf("no module domain or app packages found")
	}
	args := append([]string{"test", "-coverprofile", "coverage.out"}, packages...)
	// #nosec G204 -- arguments are package names returned by go list for this repository.
	test := exec.Command("go", args...)
	test.Stdout, test.Stderr = os.Stdout, os.Stderr
	if err := test.Run(); err != nil {
		return fmt.Errorf("run module coverage tests: %w", err)
	}
	profile, err := os.ReadFile("coverage.out")
	if err != nil {
		return fmt.Errorf("read coverage profile: %w", err)
	}
	counts := make(map[string]count)
	scanner := bufio.NewScanner(bytes.NewReader(profile))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}
		file := strings.SplitN(fields[0], ":", 2)[0]
		statements, err := strconv.Atoi(fields[1])
		if err != nil {
			return fmt.Errorf("parse statement count: %w", err)
		}
		hits, err := strconv.Atoi(fields[2])
		if err != nil {
			return fmt.Errorf("parse hit count: %w", err)
		}
		pkg := path.Dir(file)
		value := counts[pkg]
		value.total += statements
		if hits > 0 {
			value.covered += statements
		}
		counts[pkg] = value
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan coverage profile: %w", err)
	}
	sort.Strings(packages)
	var failed bool
	for _, pkg := range packages {
		value := counts[pkg]
		if value.total == 0 {
			continue
		}
		percent := 100 * float64(value.covered) / float64(value.total)
		fmt.Printf("%s: %.1f%%\n", pkg, percent)
		if percent < 80 {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("module domain/app coverage is below 80%%")
	}
	return nil
}
