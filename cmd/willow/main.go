package main

import (
	"context"
	"fmt"
	"os"

	"github.com/iamrajjoshi/willow/internal/cli"
)

func main() {
	root := cli.NewApp()
	if err := root.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
