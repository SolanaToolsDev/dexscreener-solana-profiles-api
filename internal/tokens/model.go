package tokens

// Dexscreener-compatible output shape with omitempty
type Link struct {
	Type  string `json:"type,omitempty"`
	Label string `json:"label,omitempty"`
	URL   string `json:"url,omitempty"`
}

type ProfileOut struct {
	URL          string `json:"url,omitempty"`
	ChainID      string `json:"chainId,omitempty"`
	TokenAddress string `json:"tokenAddress,omitempty"`
	Icon         string `json:"icon,omitempty"`
	Header       string `json:"header,omitempty"`
	OpenGraph    string `json:"openGraph,omitempty"`
	Description  string `json:"description,omitempty"`
	Links        []Link `json:"links,omitempty"`
}
