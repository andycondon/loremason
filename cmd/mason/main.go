// Command mason stores, checks and queries a loremason context graph.
package main

import (
	"os"

	"github.com/andycondon/loremason/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
