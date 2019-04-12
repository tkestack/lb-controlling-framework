package main

import (
	"fmt"
	"os"

	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app"
	"git.tencent.com/tke/lb-controlling-framework/pkg/logs"
)

func main() {
	command := app.NewServer()
	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
