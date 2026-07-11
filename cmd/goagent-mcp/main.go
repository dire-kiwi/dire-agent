package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"goagentcli/client"
	"goagentcli/mcpserver"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "goagent-mcp:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("goagent-mcp", flag.ContinueOnError)
	daemonURL := flags.String("daemon", "ws://127.0.0.1:7331/ws", "goagent daemon WebSocket URL")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	backend, err := client.Dial(ctx, *daemonURL)
	if err != nil {
		return err
	}
	defer backend.Close()
	server, err := mcpserver.New(backend)
	if err != nil {
		return err
	}
	return server.RunStdio(ctx)
}
