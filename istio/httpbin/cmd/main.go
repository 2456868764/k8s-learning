package main

import (
	"fmt"
	"httpbin/pkg/logs"
	"os"

	"httpbin/cmd/app"
	"httpbin/pkg/signals"
)

func main() {
	logs.InitLogger()
	ctx := signals.SetupSignalHandler()

	if err := app.NewAppCommand(ctx).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
