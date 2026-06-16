package cmd

import (
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/cloudbase/garm-agent/service"
	"github.com/spf13/cobra"
)

var (
	defaultAgentConfig = "C:\\garm-agent\\agent.toml"
	defaultServiceName = "garm-agent"
	serviceName        string
)

func runService(service *service.Service) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("failed to determine if running as service: %w", err)
	}

	if isService {
		if err := svc.Run(serviceName, service); err != nil {
			return fmt.Errorf("failed to run service: %w", err)
		}
	} else {
		if err := service.Start(); err != nil {
			return fmt.Errorf("failed to start service: %w", err)
		}

		<-service.Done()
	}
	return nil
}

func registerService(serviceName, configFile string) error {
	p, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	c := mgr.Config{
		ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		DisplayName:  serviceName,
		Description:  "GARM runner agent",
	}

	s, err := m.CreateService(serviceName, p, c, "daemon", "--config", configFile)
	if err != nil {
		return err
	}
	defer s.Close()

	// See http://stackoverflow.com/questions/35151052/how-do-i-configure-failure-actions-of-a-windows-service-written-in-go
	const (
		scActionNone    = 0
		scActionRestart = 1

		serviceConfigFailureActions = 2
	)

	type serviceFailureActions struct {
		ResetPeriod  uint32
		RebootMsg    *uint16
		Command      *uint16
		ActionsCount uint32
		Actions      uintptr
	}

	type scAction struct {
		Type  uint32
		Delay uint32
	}
	t := []scAction{
		{Type: scActionRestart, Delay: uint32(15 * time.Second / time.Millisecond)},
		{Type: scActionRestart, Delay: uint32(15 * time.Second / time.Millisecond)},
		{Type: scActionNone},
	}
	lpInfo := serviceFailureActions{ResetPeriod: uint32(24 * time.Hour / time.Second), ActionsCount: uint32(3), Actions: uintptr(unsafe.Pointer(&t[0]))}
	err = windows.ChangeServiceConfig2(s.Handle, serviceConfigFailureActions, (*byte)(unsafe.Pointer(&lpInfo)))
	// Keep t alive across the syscall so the GC cannot collect the backing
	// array while ChangeServiceConfig2 is reading through the uintptr.
	runtime.KeepAlive(t)
	if err != nil {
		return err
	}

	return nil
}

var serviceRegisterCmd = &cobra.Command{
	Use:          "register",
	SilenceUsage: false,
	Short:        "Register the GARM agent as a service",
	Long:         `Register the GARM agent as a service.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		if serviceName == "" || agentConfig == "" {
			return fmt.Errorf("missing service name or agent config")
		}
		return registerService(serviceName, agentConfig)
	},
}

func init() {
	serviceRegisterCmd.Flags().StringVar(&agentConfig, "config", defaultAgentConfig, "The GARM agent configuration file.")
	serviceRegisterCmd.Flags().StringVar(&serviceName, "service-name", defaultServiceName, "The default service name.")
	rootCmd.AddCommand(serviceRegisterCmd)
}
