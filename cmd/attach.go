package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/executor"
)

var attachCmd = &cobra.Command{
	Use:   "attach <connector>",
	Short: "Attach to a running connector's data stream",
	Long:  "Connect to an existing connector daemon via Unix Domain Socket and stream NDJSON messages.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		conn, err := executor.AttachConnector(name)
		if err != nil {
			return err
		}
		defer conn.Close()

		fmt.Fprintf(os.Stderr, "Attached to %q. Press Ctrl+C to detach.\n", name)

		// Handle Ctrl+C
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		// Stream messages to stdout
		doneCh := make(chan struct{})
		go func() {
			defer close(doneCh)
			scanner := bufio.NewScanner(conn)
			for scanner.Scan() {
				fmt.Println(scanner.Text())
			}
		}()

		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "\nDetached.")
		case <-doneCh:
			fmt.Fprintln(os.Stderr, "Connection closed.")
		}

		return nil
	},
}
