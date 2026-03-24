package cmd

import (
	"fmt"
	runtimeDebug "runtime/debug"

	"github.com/spf13/cobra"
)

var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the installed rog version",
	Args:  cobra.NoArgs,
	Run:   runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println(resolveVersion())
}

func resolveVersion() string {
	if version != "" && version != "dev" {
		return version
	}

	buildInfo, ok := runtimeDebug.ReadBuildInfo()
	if !ok || buildInfo.Main.Version == "" || buildInfo.Main.Version == "(devel)" {
		return version
	}

	return buildInfo.Main.Version
}
