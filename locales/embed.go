package locales

import "embed"

// FS contains the embedded locale catalogs used by the i18n package.
//
//go:embed *.yaml
var FS embed.FS
