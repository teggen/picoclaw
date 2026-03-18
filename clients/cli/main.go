package main

import (
	"fmt"
	"os"

	"github.com/sipeed/picoclaw/clients/cli/internal"
)

func main() {
	if err := internal.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
