package main

import (
	"fmt"
	"os"

	"github.com/dlorenc/multiclaude/internal/cli"
	"github.com/dlorenc/multiclaude/internal/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, errors.Format(err))
		os.Exit(1)
	}
}

func run() error {
	c, err := cli.New()
	if err != nil {
		return err
	}

	return c.Execute(os.Args[1:])
}
