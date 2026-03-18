package configcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
	"github.com/sipeed/picoclaw/clients/cli/internal/styles"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration management",
	}

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get current configuration",
		RunE:  runGet,
	}

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Replace entire configuration",
		RunE:  runSet,
	}
	setCmd.Flags().String("file", "", "Read config from file (- for stdin)")

	patchCmd := &cobra.Command{
		Use:   "patch [json]",
		Short: "Patch configuration (JSON merge patch)",
		RunE:  runPatch,
	}
	patchCmd.Flags().String("file", "", "Read patch from file (- for stdin)")

	cmd.AddCommand(getCmd, setCmd, patchCmd)
	return cmd
}

func runGet(cmd *cobra.Command, _ []string) error {
	c, jsonMode, _ := client.FromCommand(cmd)

	raw, err := c.GetConfig()
	if err != nil {
		return err
	}

	if jsonMode {
		fmt.Println(string(raw))
		return nil
	}

	// Pretty-print JSON with syntax coloring.
	var buf bytes.Buffer
	if json.Indent(&buf, raw, "", "  ") != nil {
		fmt.Println(string(raw))
		return nil
	}

	content := colorizeJSON(buf.String())

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(0, 1).
		Render(content)

	fmt.Println(box)
	return nil
}

func runSet(cmd *cobra.Command, _ []string) error {
	c, _, _ := client.FromCommand(cmd)

	data, err := readInput(cmd)
	if err != nil {
		return err
	}

	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON input")
	}

	if err := c.PutConfig(data); err != nil {
		return err
	}

	fmt.Println(styles.ValueStyle.Render("Configuration updated."))
	return nil
}

func runPatch(cmd *cobra.Command, args []string) error {
	c, _, _ := client.FromCommand(cmd)

	var data []byte
	var err error

	if len(args) > 0 {
		data = []byte(args[0])
	} else {
		data, err = readInput(cmd)
		if err != nil {
			return err
		}
	}

	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON input")
	}

	if err := c.PatchConfig(data); err != nil {
		return err
	}

	fmt.Println(styles.ValueStyle.Render("Configuration patched."))
	return nil
}

func readInput(cmd *cobra.Command) ([]byte, error) {
	filePath, _ := cmd.Flags().GetString("file")
	if filePath == "" {
		// Check if stdin has data.
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			return io.ReadAll(os.Stdin)
		}
		return nil, fmt.Errorf("provide input via --file, stdin pipe, or as argument")
	}

	if filePath == "-" {
		return io.ReadAll(os.Stdin)
	}

	return os.ReadFile(filePath)
}

func colorizeJSON(s string) string {
	keyStyle := lipgloss.NewStyle().Foreground(styles.Primary)
	strStyle := lipgloss.NewStyle().Foreground(styles.Success)
	numStyle := lipgloss.NewStyle().Foreground(styles.Secondary)
	boolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))

	var result []byte
	data := []byte(s)
	i := 0

	for i < len(data) {
		switch {
		case data[i] == '"':
			// Find end of string.
			end := i + 1
			for end < len(data) && data[end] != '"' {
				if data[end] == '\\' {
					end++
				}
				end++
			}
			if end < len(data) {
				end++
			}
			str := string(data[i:end])
			// Check if this is a key (followed by ':').
			rest := bytes.TrimLeft(data[end:], " \t\n\r")
			if len(rest) > 0 && rest[0] == ':' {
				result = append(result, []byte(keyStyle.Render(str))...)
			} else {
				result = append(result, []byte(strStyle.Render(str))...)
			}
			i = end
		case data[i] >= '0' && data[i] <= '9' || data[i] == '-':
			end := i + 1
			for end < len(data) && (data[end] >= '0' && data[end] <= '9' || data[end] == '.' || data[end] == 'e' || data[end] == 'E' || data[end] == '+' || data[end] == '-') {
				end++
			}
			result = append(result, []byte(numStyle.Render(string(data[i:end])))...)
			i = end
		case bytes.HasPrefix(data[i:], []byte("true")):
			result = append(result, []byte(boolStyle.Render("true"))...)
			i += 4
		case bytes.HasPrefix(data[i:], []byte("false")):
			result = append(result, []byte(boolStyle.Render("false"))...)
			i += 5
		case bytes.HasPrefix(data[i:], []byte("null")):
			result = append(result, []byte(styles.MutedStyle.Render("null"))...)
			i += 4
		default:
			result = append(result, data[i])
			i++
		}
	}

	return string(result)
}
