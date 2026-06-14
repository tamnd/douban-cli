package douban

import "context"

// The input structs below are reflected by kit to build CLI flags, HTTP query
// params, and MCP tool arguments. Each carries the injected *Client and, where a
// command has a default fetch count, an inherited --limit so the handler can
// size its request. kit enforces the row limit again around emit.

type searchIn struct {
	Query  string  `kit:"arg" help:"search query"`
	Type   string  `kit:"flag,short=t" default:"book" enum:"book,movie,music" help:"content type: book, movie, or music"`
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

func search(ctx context.Context, in searchIn, emit func(Result) error) error {
	results, err := in.Client.Search(ctx, in.Query, in.Type, limitOr(in.Limit, 15))
	if err != nil {
		return mapErr(err)
	}
	for _, r := range results {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

type suggestIn struct {
	Query  string  `kit:"arg" help:"search query"`
	Type   string  `kit:"flag,short=t" default:"movie" enum:"movie,book" help:"content type: movie or book"`
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

func suggest(ctx context.Context, in suggestIn, emit func(Suggestion) error) error {
	results, err := in.Client.Suggest(ctx, in.Query, in.Type, limitOr(in.Limit, 10))
	if err != nil {
		return mapErr(err)
	}
	for _, s := range results {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

type bookIn struct {
	Ref    string  `kit:"arg" help:"book id or book.douban.com URL"`
	Client *Client `kit:"inject"`
}

func getBook(ctx context.Context, in bookIn, emit func(*Book) error) error {
	b, err := in.Client.Book(ctx, in.Ref)
	if err != nil {
		return mapErr(err)
	}
	return emit(b)
}

type top250In struct {
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

func top250(ctx context.Context, in top250In, emit func(Movie) error) error {
	films, err := in.Client.Top250(ctx, limitOr(in.Limit, 250))
	if err != nil {
		return mapErr(err)
	}
	for _, m := range films {
		if err := emit(m); err != nil {
			return err
		}
	}
	return nil
}

type chartIn struct {
	Client *Client `kit:"inject"`
}

func chart(ctx context.Context, in chartIn, emit func(Movie) error) error {
	films, err := in.Client.Chart(ctx)
	if err != nil {
		return mapErr(err)
	}
	for _, m := range films {
		if err := emit(m); err != nil {
			return err
		}
	}
	return nil
}

type nowplayingIn struct {
	City   string  `kit:"flag" default:"beijing" help:"city slug (e.g. beijing, shanghai)"`
	Coming bool    `kit:"flag" help:"show the upcoming list instead of now playing"`
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

func nowplaying(ctx context.Context, in nowplayingIn, emit func(Movie) error) error {
	city := in.City
	if city == "" {
		city = "beijing"
	}
	films, err := in.Client.NowPlaying(ctx, city, in.Coming, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, m := range films {
		if err := emit(m); err != nil {
			return err
		}
	}
	return nil
}

type doulistIn struct {
	Ref    string  `kit:"arg" help:"doulist id or www.douban.com/doulist URL"`
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

func doulist(ctx context.Context, in doulistIn, emit func(DoulistItem) error) error {
	items, err := in.Client.Doulist(ctx, in.Ref, limitOr(in.Limit, 100))
	if err != nil {
		return mapErr(err)
	}
	for _, it := range items {
		if err := emit(it); err != nil {
			return err
		}
	}
	return nil
}
