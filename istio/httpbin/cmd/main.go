package main

import (
	"fmt"
	"os"

	"httpbin/cmd/app"
	"httpbin/pkg/logs"
	"httpbin/pkg/signals"
)

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	ctx := signals.SetupSignalHandler()

	if err := app.NewAppCommand(ctx).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
