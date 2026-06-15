//go:build !linux

package console

import (
	"log/slog"

	"github.com/spf13/cobra"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type MetricsCgroup struct {
	console2.Abstract
}

func (c MetricsCgroup) GetName() string {
	return "metrics:cgroup"
}

func (c MetricsCgroup) Configure(cmd *cobra.Command) {
}

func (c MetricsCgroup) GetDescription() string {
	return "cgroup metrics cgroup"
}

func (c MetricsCgroup) Handle(cmd *cobra.Command, args []string) {
	slog.Error("cgroup metrics is only supported on linux")
}
