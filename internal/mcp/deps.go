package mcp

import (
	"context"
	"net"
	"time"

	"github.com/YangZhengCQ/Claw2cli/internal/executor"
	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

// Package-level function pointers for dependency injection in tests.
var (
	runSkillFn           = func(ctx context.Context, manifest *parser.PluginManifest, args []string, timeout time.Duration) (*executor.SkillResult, error) {
		return executor.RunSkill(ctx, manifest, args, timeout)
	}
	startConnectorFn     = func(manifest *parser.PluginManifest) error {
		return executor.StartConnector(manifest)
	}
	stopConnectorFn      = func(name string) error {
		return executor.StopConnector(name)
	}
	getConnectorStatusFn = func(name string) (*executor.ConnectorStatus, error) {
		return executor.GetConnectorStatus(name)
	}
	attachConnectorFn    = func(name string) (net.Conn, error) {
		return executor.AttachConnector(name)
	}
)
