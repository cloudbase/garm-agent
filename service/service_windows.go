package service

import (
	"log/slog"

	"golang.org/x/sys/windows/svc"
)

func (s *Service) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	if err := s.Start(); err != nil {
		return false, 11
	}
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	slog.InfoContext(s.ctx, "service is starting")

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				slog.InfoContext(s.ctx, "service is stopping")
				break loop
			default:
				slog.InfoContext(s.ctx, "Unexpected control request", "request", c.Cmd)
			}
		case <-s.done:
			break loop
		case <-s.ctx.Done():
			break loop
		}
	}

	status <- svc.Status{State: svc.StopPending}
	if stopErr := s.Stop(); stopErr != nil {
		slog.ErrorContext(s.ctx, "failed to stop service", "error", stopErr)
	}
	// Wait for the service goroutines (including the runner cleanup) to finish
	// before reporting the service stopped.
	s.Wait()
	return false, 0
}
