// Command patchcord is the entry point of the Patchcord agent binary.
package main

import (
	"fmt"
	"os"

	"github.com/lucasglmt/patchcord/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
