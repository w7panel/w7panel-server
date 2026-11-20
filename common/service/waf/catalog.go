package waf

import (
	_ "embed"
	"encoding/json"
)

type CatalogFile struct {
	Name    string `json:"name"`
	TitleZh string `json:"titleZh"`
}

type Catalog struct {
	CRSVersion string            `json:"crsVersion"`
	Files      []CatalogFile     `json:"files"`
	Rules      []json.RawMessage `json:"rules"`
}

//go:embed catalog.json
var catalogJSON []byte

var SystemCatalog Catalog
var validRuleFiles = map[string]bool{}

func init() {
	if err := json.Unmarshal(catalogJSON, &SystemCatalog); err != nil {
		panic(err)
	}
	for _, file := range SystemCatalog.Files {
		validRuleFiles[file.Name] = true
	}
}
