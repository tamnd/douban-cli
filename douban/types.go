package douban

// doubanData is the top-level wrapper decoded from window.__DATA__.
type doubanData struct {
	Items []doubanItem `json:"items"`
	Total int          `json:"total"`
	Count int          `json:"count"`
}

// doubanItem is one entry in the items array.
type doubanItem struct {
	TplName  string `json:"tpl_name"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Abstract string `json:"abstract"`
	Rating   struct {
		Value     float64 `json:"value"`
		Count     int     `json:"count"`
		StarCount float64 `json:"star_count"`
	} `json:"rating"`
}

// Result is the record emitted for every matched search result.
type Result struct {
	Rank     int    `json:"rank"`
	Title    string `json:"title"`
	Rating   string `json:"rating"`
	Abstract string `json:"abstract"`
	URL      string `json:"url"`
}
