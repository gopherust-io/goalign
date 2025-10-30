package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:
$ source <(goalign completion bash)

# To load completions for each session, execute once:
Linux:
  $ goalign completion bash > /etc/bash_completion.d/goalign
MacOS:
  $ goalign completion bash > /usr/local/etc/bash_completion.d/goalign

Zsh:
$ source <(goalign completion zsh)

# To load completions for each session, execute once:
$ goalign completion zsh > "${fpath[1]}/_goalign"

Fish:
$ goalign completion fish | source

# To load completions for each session, execute once:
$ goalign completion fish > ~/.config/fish/completions/goalign.fish
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletion(os.Stdout)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
