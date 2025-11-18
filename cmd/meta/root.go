package meta

import (
	"github.com/spf13/cobra"
)

var MetaCmd = &cobra.Command{
	Use:   "meta",
	Short: "Manage repository metadata",
	Long: `Manage repository metadata files (.rogmeta.yml and global meta.yml).

Metadata precedence:
  1. .rogmeta.yml (per-repository, manual)
  2. global meta.yml (manual)
  3. LLM-generated
  4. Auto-detected`,
}

func init() {
	MetaCmd.AddCommand(initCmd)
	MetaCmd.AddCommand(editCmd)
}
