// Command rzv is the Runzivo command-line tool.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Runzivo/rzv-cli/internal/browser"
	"github.com/Runzivo/rzv-cli/internal/cli"
	"github.com/Runzivo/rzv-cli/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	deps := cli.Deps{
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Getenv:      os.Getenv,
		OpenBrowser: browser.Open,
	}
	if dir, err := config.Directory(os.Getenv); err == nil {
		deps.Store = &config.Store{Dir: dir}
	} else {
		fmt.Fprintf(os.Stderr, "rzv: %s\n", err)
	}
	os.Exit(cli.Run(ctx, os.Args[1:], deps))
}
