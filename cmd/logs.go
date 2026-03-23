package cmd

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/YangZhengCQ/Claw2cli/internal/paths"
)

var followLogs bool

var logsCmd = &cobra.Command{
	Use:   "logs <connector>",
	Short: "Tail logs from a running connector",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		logPath := filepath.Join(paths.BaseDir(), "logs", name+".log")

		f, err := os.Open(logPath)
		if err != nil {
			return fmt.Errorf("open logs for %q: %w", name, err)
		}
		defer f.Close()

		if !followLogs {
			// Just dump the log
			io.Copy(os.Stdout, f)
			return nil
		}

		// Follow mode: tail -f
		// First dump existing content
		io.Copy(os.Stdout, f)

		// Handle Ctrl+C
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		// Then poll for new content
		for {
			select {
			case <-sigCh:
				return nil
			default:
			}
			n, err := io.Copy(os.Stdout, f)
			if err != nil {
				return err
			}
			if n == 0 {
				time.Sleep(200 * time.Millisecond)
			}
		}
	},
}

func init() {
	logsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output")
}
