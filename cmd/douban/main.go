// Command douban is the single-binary command line for douban-cli. It hands the
// kit App to kit.Run, which builds the CLI, the HTTP API (douban serve), and the
// MCP server (douban mcp) from the one operation registry and maps errors to
// stable exit codes.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/douban-cli/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(kit.Run(ctx, cli.NewApp()))
}
