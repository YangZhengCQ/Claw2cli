package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/paths"
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
			return fmt.Errorf("no logs found for %q", name)
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

		// Then poll for new content
		for {
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
