package main

import (
	"context"
	"os"

	"github.com/iamrajjoshi/willow/internal/cli"
	"github.com/iamrajjoshi/willow/internal/ui"
)

func main() {
	root := cli.NewApp()
	if err := root.Run(context.Background(), os.Args); err != nil {
		u := &ui.UI{NoColor: root.Bool("no-color")}
		u.Errorf("%v", err)
		os.Exit(1)
	}
}
