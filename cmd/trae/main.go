package main

import (
	"fmt"
	"os"

	"github.com/bytedance/trae-agent/internal/cli"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "trae: internal error: %v\n", r)
			os.Exit(2)
		}
	}()
	root := cli.NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "trae: %v\n", err)
		os.Exit(1)
	}
}
