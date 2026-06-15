//go:build !windows
// +build !windows

package cmd

import (
	"fmt"

	"github.com/cloudbase/garm-agent/service"
)

var defaultAgentConfig = "/etc/garm/agent.toml"

func runService(service *service.Service) error {
	if err := service.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	<-service.Done()
	// Wait for the service goroutines to finish so the runner's process group is
	// torn down before we exit; otherwise the runner is orphaned on shutdown.
	service.Wait()
	return nil
}
