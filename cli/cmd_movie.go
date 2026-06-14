package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) top250Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "top250",
		Short: "The Douban Top 250 films, in rank order",
		Long: `List the Douban Top 250 films, in rank order.

Records include rank, id, title, aka, year, region, genres, directors, cast,
rating, votes, quote, and URL. Use -n to fetch fewer than 250.

Examples:
  douban top250
  douban top250 -n 20
  douban top250 -o csv > top250.csv`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(250)
			a.progressf("fetching top 250 (limit %d)...", n)
			films, err := a.client.Top250(cmd.Context(), n)
			if err != nil {
				return a.fail(err)
			}
			return a.renderOrEmpty(films, len(films))
		},
	}
}

func (a *App) chartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chart",
		Short: "The current Douban film ranking board",
		Long: `List the current Douban film ranking board (/chart).

Records include id, title, aka, cast, rating, votes, and URL.

Examples:
  douban chart
  douban chart -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a.progressf("fetching film chart...")
			films, err := a.client.Chart(cmd.Context())
			if err != nil {
				return a.fail(err)
			}
			return a.renderOrEmpty(films, len(films))
		},
	}
}

func (a *App) nowplayingCmd() *cobra.Command {
	var city string
	var coming bool

	cmd := &cobra.Command{
		Use:   "nowplaying",
		Short: "Films in cinemas now (or upcoming) for a city",
		Long: `List the films in cinemas now for a city, or the upcoming list.

Records include id, title, year, region, genres, directors, cast, duration,
rating, votes, and URL.

Examples:
  douban nowplaying
  douban nowplaying --city shanghai
  douban nowplaying --coming
  douban nowplaying -o jsonl | jq -r '.title'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(0)
			what := "now playing"
			if coming {
				what = "coming soon"
			}
			a.progressf("fetching %s in %s...", what, city)
			films, err := a.client.NowPlaying(cmd.Context(), city, coming, n)
			if err != nil {
				return a.fail(err)
			}
			return a.renderOrEmpty(films, len(films))
		},
	}

	cmd.Flags().StringVar(&city, "city", "beijing", "city slug (e.g. beijing, shanghai)")
	cmd.Flags().BoolVar(&coming, "coming", false, "show the upcoming list instead of now playing")
	return cmd
}
