package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/YangZhengCQ/Claw2cli/internal/executor"
	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
)

var echoCmd = &cobra.Command{
	Use:   "echo <connector>",
	Short: "Test consumer that echoes back received messages",
	Long: `Connect to a running connector's UDS and act as a test consumer.
When a message is received, automatically sends back an echo reply.
Useful for verifying the full bidirectional message flow.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		conn, err := executor.AttachConnector(name)
		if err != nil {
			return err
		}
		defer conn.Close()

		fmt.Fprintf(os.Stderr, "Echo consumer attached to %q. Waiting for messages...\n", name)

		// Handle Ctrl+C
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		doneCh := make(chan struct{})
		go func() {
			defer close(doneCh)
			scanner := bufio.NewScanner(conn)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			for scanner.Scan() {
				line := scanner.Bytes()

				var msg protocol.Message
				if err := json.Unmarshal(line, &msg); err != nil {
					continue
				}

				switch msg.Type {
				case protocol.TypeEvent:
					if msg.Topic == "message.received" {
						var p struct {
							From string `json:"from"`
							Body string `json:"body"`
						}
						if json.Unmarshal(msg.Payload, &p) == nil {
							fmt.Fprintf(os.Stderr, "📨 Message from %s: %s\n", p.From, p.Body)
						}
					}

				case protocol.TypeCommand:
					if msg.Action == "get_reply" {
						var p struct {
							From string `json:"from"`
							Body string `json:"body"`
						}
						if json.Unmarshal(msg.Payload, &p) == nil {
							fmt.Fprintf(os.Stderr, "🔄 Replying to %s...\n", p.From)
						}

						// Build echo reply
						replyText := fmt.Sprintf("[c2c echo] 收到你的消息，时间: %s", time.Now().Format("15:04:05"))
						if len(p.Body) > 0 {
							replyText = fmt.Sprintf("[c2c echo] 你说了: %s", p.Body)
						}

						resp := protocol.Message{
							Type:   protocol.TypeResponse,
							Source: name,
							ID:     msg.ID,
						}
						payload, err := json.Marshal(map[string]string{"text": replyText})
						if err != nil {
							fmt.Fprintf(os.Stderr, "marshal reply: %v\n", err)
							continue
						}
						resp.Payload = payload

						data, err := json.Marshal(resp)
						if err != nil {
							fmt.Fprintf(os.Stderr, "marshal response: %v\n", err)
							continue
						}
						data = append(data, '\n')
						if _, err := conn.Write(data); err != nil {
							fmt.Fprintf(os.Stderr, "write reply: %v\n", err)
							return
						}
						fmt.Fprintf(os.Stderr, "✅ Sent: %s\n", replyText)
					}

				case protocol.TypeLog:
					fmt.Fprintf(os.Stderr, "[%s] %s\n", msg.Level, msg.MessageStr)
				}
			}
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			}
		}()

		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "\nEcho consumer detached.")
		case <-doneCh:
			fmt.Fprintln(os.Stderr, "Connection closed.")
		}

		return nil
	},
}
