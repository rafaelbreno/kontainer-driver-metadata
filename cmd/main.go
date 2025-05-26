package main

import (
	"fmt"
	"os"

	"github.com/rancher/kontainer-driver-metadata/pkg/rke2"
	"go.uber.org/zap"
)

var logger *zap.Logger

func main() {
	// Initialize Zap logger
	var initErr error
	logger, initErr = zap.NewDevelopment()
	if initErr != nil {
		fmt.Printf("Failed to initialize zap logger: %v\n", initErr)
		os.Exit(1)
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	if err := rke2.UpdateRKE2(""); err != nil {
		logger.Fatal("Error updating releases sequence node", zap.Error(err))
	}
}
