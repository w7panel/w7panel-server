package waf

import (
	_ "embed"
	"encoding/json"
)

type CatalogFile struct {
	Name    string `json:"name"`
	TitleZh string `json:"titleZh"`
}

type catalogRule struct {
	ID            int    `json:"id"`
	File          string `json:"file"`
	Category      int    `json:"category"`
	Message       string `json:"message"`
	ParanoiaLevel *int   `json:"paranoiaLevel"`
	Internal      bool   `json:"internal"`
	NameZh        string `json:"nameZh"`
}
type Catalog struct {
	CRSVersion string        `json:"crsVersion"`
	Files      []CatalogFile `json:"files"`
	Rules      []catalogRule `json:"rules"`
}

//go:embed catalog.json
var catalogJSON []byte

var SystemCatalog Catalog
var validRuleFiles = map[string]bool{}
var validDetectionIDs = map[int]string{}

func init() {
	if err := json.Unmarshal(catalogJSON, &SystemCatalog); err != nil {
		panic(err)
	}
	for _, file := range SystemCatalog.Files {
		validRuleFiles[file.Name] = true
	}
	for _, rule := range SystemCatalog.Rules {
		if !rule.Internal && rule.Message != "" {
			validDetectionIDs[rule.ID] = rule.File
		}
	}
}
