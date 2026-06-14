package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) searchCmd() *cobra.Command {
	var searchType string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search Douban for books, movies, or music",
		Long: `Search Douban (豆瓣) for books, movies, or music.

Results include rank, title, rating, abstract, and URL.

Examples:
  douban search python
  douban search "流浪地球" --type movie
  douban search "周杰伦" --type music --limit 5
  douban search python -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := a.effectiveLimit(15)
			a.progressf("searching douban %s for %q...", searchType, args[0])
			results, err := a.client.Search(cmd.Context(), args[0], searchType, n)
			if err != nil {
				return codeError(exitError, err)
			}
			return a.renderOrEmpty(results, len(results))
		},
	}

	cmd.Flags().StringVarP(&searchType, "type", "t", "book", "content type: book, movie, or music")
	return cmd
}
