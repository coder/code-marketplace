package main

import (
	"fmt"
	"os"

	"github.com/coder/code-marketplace/cli"
)

func main() {
	err := cli.Root().Execute()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
