//go:build !linux && !windows
// +build !linux,!windows

package service

import (
	"fmt"

	"github.com/cloudbase/garm-agent/config"
)

var _ PTY = &sessionPTY{}

func NewSessionPTY(_ *config.Agent) (PTY, error) {
	return &sessionPTY{}, nil
}

type sessionPTY struct {
}

func (p *sessionPTY) Read([]byte) (int, error) {
	return 0, fmt.Errorf("unsupported")
}

func (p *sessionPTY) Write([]byte) (int, error) {
	return 0, fmt.Errorf("unsupported")
}

func (p *sessionPTY) Resize(cols, rows uint16) error {
	return fmt.Errorf("unsupported")
}

func (p *sessionPTY) Close() error {
	return fmt.Errorf("unsupported")

}

func DefaultShell() (string, error) {
	return "", fmt.Errorf("unsupported")
}
