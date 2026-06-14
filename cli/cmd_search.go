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

Results include rank, id, type, title, rating, abstract, and URL.

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
				return a.fail(err)
			}
			return a.renderOrEmpty(results, len(results))
		},
	}

	cmd.Flags().StringVarP(&searchType, "type", "t", "book", "content type: book, movie, or music")
	return cmd
}

func (a *App) suggestCmd() *cobra.Command {
	var suggestType string

	cmd := &cobra.Command{
		Use:   "suggest <query>",
		Short: "Fast typeahead lookup (the reliable way to find a film's id)",
		Long: `Query Douban's typeahead (j/subject_suggest) for a quick lookup.

Records include id, type, title, sub_title, year, and URL. Movie suggest is the
reliable path to a film's id, since movie subject pages are sealed behind an
anti-bot challenge.

Examples:
  douban suggest matrix
  douban suggest 三体 --type book
  douban suggest matrix -o jsonl | jq -r '.id'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := a.effectiveLimit(10)
			a.progressf("suggesting douban %s for %q...", suggestType, args[0])
			results, err := a.client.Suggest(cmd.Context(), args[0], suggestType, n)
			if err != nil {
				return a.fail(err)
			}
			return a.renderOrEmpty(results, len(results))
		},
	}

	cmd.Flags().StringVarP(&suggestType, "type", "t", "movie", "content type: movie or book")
	return cmd
}
