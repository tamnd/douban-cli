package douban

// --- search (window.__DATA__) ---

// doubanData is the top-level wrapper decoded from window.__DATA__.
type doubanData struct {
	Items []doubanItem `json:"items"`
	Total int          `json:"total"`
	Count int          `json:"count"`
}

// doubanItem is one entry in the search items array.
type doubanItem struct {
	TplName  string `json:"tpl_name"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Abstract string `json:"abstract"`
	CoverURL string `json:"cover_url"`
	Rating   struct {
		Value     float64 `json:"value"`
		Count     int     `json:"count"`
		StarCount float64 `json:"star_count"`
	} `json:"rating"`
}

// Result is the record emitted for every search hit.
type Result struct {
	Rank     int    `json:"rank"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Rating   string `json:"rating"`
	Abstract string `json:"abstract" table:",truncate"`
	URL      string `json:"url"`
	Cover    string `json:"cover,omitempty" table:",truncate"`
}

// --- suggest (j/subject_suggest) ---

// wireSuggest is one entry in the suggest JSON array.
type wireSuggest struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	SubTitle string `json:"sub_title"`
	Year     string `json:"year"`
	URL      string `json:"url"`
	Img      string `json:"img"`
	Episode  string `json:"episode"`
}

// Suggestion is the record emitted for every suggest hit.
type Suggestion struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	SubTitle string `json:"sub_title,omitempty"`
	Year     string `json:"year,omitempty"`
	URL      string `json:"url"`
	Img      string `json:"img,omitempty" table:",truncate"`
}

// --- book subject ---

// Book is the full record for one book subject.
type Book struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title,omitempty"`
	Author        string `json:"author,omitempty"`
	Translator    string `json:"translator,omitempty"`
	Publisher     string `json:"publisher,omitempty"`
	Producer      string `json:"producer,omitempty"`
	PubDate       string `json:"pubdate,omitempty"`
	Pages         string `json:"pages,omitempty"`
	Price         string `json:"price,omitempty"`
	Binding       string `json:"binding,omitempty"`
	Series        string `json:"series,omitempty"`
	ISBN          string `json:"isbn,omitempty"`
	Rating        string `json:"rating"`
	Summary       string `json:"summary,omitempty" table:",truncate"`
	URL           string `json:"url"`
	Cover         string `json:"cover,omitempty" table:",truncate"`
}

// --- movie rows (top250 / chart / coming / nowplaying) ---

// Movie is a unified film record; fields a given surface does not carry are
// omitted from output.
type Movie struct {
	Rank      int    `json:"rank,omitempty"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	AKA       string `json:"aka,omitempty"`
	Year      string `json:"year,omitempty"`
	Region    string `json:"region,omitempty"`
	Genres    string `json:"genres,omitempty"`
	Directors string `json:"directors,omitempty"`
	Cast      string `json:"cast,omitempty" table:",truncate"`
	Duration  string `json:"duration,omitempty"`
	Rating    string `json:"rating"`
	URL       string `json:"url"`
	Cover     string `json:"cover,omitempty" table:",truncate"`
	Quote     string `json:"quote,omitempty" table:",truncate"`
}

// --- doulist ---

// DoulistItem is one subject in a curated list.
type DoulistItem struct {
	Rank     int    `json:"rank"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	Rating   string `json:"rating"`
	Abstract string `json:"abstract,omitempty" table:",truncate"`
	URL      string `json:"url"`
	Cover    string `json:"cover,omitempty" table:",truncate"`
}
