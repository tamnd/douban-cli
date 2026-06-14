package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) bookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "book <id|url>",
		Short: "Show full detail for one book",
		Long: `Fetch the full record for one book subject.

The argument is a numeric subject id or a pasted book.douban.com URL. Fields
include author, translator, publisher, pubdate, pages, price, isbn, rating,
votes, and a summary.

Examples:
  douban book 1084336
  douban book https://book.douban.com/subject/1084336/
  douban book 1084336 -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.progressf("fetching book %s...", args[0])
			b, err := a.client.Book(cmd.Context(), args[0])
			if err != nil {
				return a.fail(err)
			}
			return a.render(b)
		},
	}
}
