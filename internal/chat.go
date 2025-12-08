package internal

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/nyaosorg/go-readline-ny/completion"
)

// Message represents a chat message
type ChatMessage struct {
	Content   string
	FromUser  bool
	Timestamp time.Time
}

type CLIInterface struct {
	manager     *Manager
	initMessage string
}

func NewCLIInterface(manager *Manager) *CLIInterface {
	return &CLIInterface{
		manager:     manager,
		initMessage: "",
	}
}

// Start starts the CLI interface
func (c *CLIInterface) Start(initMessage string) error {
	return c.StartTUI(initMessage)
}

// printWelcomeMessage prints a welcome message
func (c *CLIInterface) printWelcomeMessage() {
	fmt.Println()
	fmt.Println("Type '/help' for a list of commands, '/exit' to quit")
	fmt.Println()
}

func (c *CLIInterface) processInput(input string) {
	if c.manager.IsMessageSubcommand(input) {
		c.manager.ProcessSubCommand(input)
		return
	}

	// Set up signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	// Set up a notification channel
	done := make(chan struct{})

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Launch a goroutine just for handling the interrupt
	go func() {
		select {
		case <-sigChan:
			cancel()
			c.manager.Status = ""
			c.manager.WatchMode = false
		case <-done:
		}
	}()

	// Run the message processing in the main thread
	c.manager.Status = "running"
	c.manager.ProcessUserMessage(ctx, input)
	c.manager.Status = ""

	close(done)

	signal.Stop(sigChan)
}

// newCompleter creates a completion handler for command completion
func (c *CLIInterface) newCompleter() *completion.CmdCompletionOrList2 {
	return &completion.CmdCompletionOrList2{
		Delimiter: " ",
		Postfix:   " ",
		Candidates: func(field []string) (forComp []string, forList []string) {
			// Handle top-level commands
			if len(field) == 0 || (len(field) == 1 && !strings.HasSuffix(field[0], " ")) {
				return commands, commands
			}

			// Handle /config subcommands
			if len(field) > 0 && field[0] == "/config" {
				if len(field) == 1 || (len(field) == 2 && !strings.HasSuffix(field[1], " ")) {
					return []string{"set", "get"}, []string{"set", "get"}
				} else if len(field) == 2 || (len(field) == 3 && !strings.HasSuffix(field[2], " ")) {
					return AllowedConfigKeys, AllowedConfigKeys
				}
			}
			return nil, nil
		},
	}
}
