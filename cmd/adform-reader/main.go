package main

import (
	"context"
	"os"

	"adform/internal/cli"
)

func main() {
	os.Exit(cli.RunReader(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
