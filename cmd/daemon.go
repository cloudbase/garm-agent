// Copyright 2025 Cloudbase Solutions SRL
//
//	Licensed under the Apache License, Version 2.0 (the "License"); you may
//	not use this file except in compliance with the License. You may obtain
//	a copy of the License at
//
//	     http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//	WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//	License for the specific language governing permissions and limitations
//	under the License.
package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cloudbase/garm-agent/config"
	"github.com/cloudbase/garm-agent/service"
)

var agentConfig = "/etc/garm/agent.toml"

var signals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
}

var daemonCmd = &cobra.Command{
	Use:          "daemon",
	SilenceUsage: false,
	Short:        "Start GARM agent",
	Long:         `Run the GARM agent.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), signals...)
		defer stop()

		cfg, err := config.NewConfig(agentConfig)
		if err != nil {
			return err
		}

		opts := slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		var logDestination io.Writer
		if cfg.LogFile == "" {
			logDestination = os.Stdout
		} else {
			fd, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			defer fd.Close()
			logDestination = fd
		}
		fileHan := slog.NewTextHandler(logDestination, &opts)
		slog.SetDefault(slog.New(fileHan))

		svc, err := service.NewService(ctx, cfg)
		if err != nil {
			return err
		}

		return runService(svc)
	},
}

func init() {
	daemonCmd.Flags().StringVar(&agentConfig, "config", defaultAgentConfig, "The GARM agent configuration file.")
	rootCmd.AddCommand(daemonCmd)
}
