// Command nota captures notes and reminders without leaving the terminal.
package main

import (
	"os"

	"github.com/dignajar/nota/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
