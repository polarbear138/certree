package main

import (
	"context"
	"fmt"
	"github.com/certree/certree/internal/agent"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := agent.CLI(ctx, os.Args[1:], os.Stdin, os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, "certree:", e)
		os.Exit(1)
	}
}
