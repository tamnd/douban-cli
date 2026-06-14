package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) doulistCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doulist <id|url>",
		Short: "List the subjects in a curated list (豆列)",
		Long: `List the subjects in a curated Douban list (豆列), in list order.

The argument is a numeric doulist id or a pasted www.douban.com/doulist URL.
Records include rank, id, title, rating, abstract, and URL. Use -n to cap the
number of items (default 100).

Examples:
  douban doulist 240962
  douban doulist https://www.douban.com/doulist/240962/
  douban doulist 240962 -n 20 -o csv`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := a.effectiveLimit(100)
			a.progressf("fetching doulist %s...", args[0])
			items, err := a.client.Doulist(cmd.Context(), args[0], n)
			if err != nil {
				return a.fail(err)
			}
			return a.renderOrEmpty(items, len(items))
		},
	}
}
