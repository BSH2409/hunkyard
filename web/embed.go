package webassets

import "embed"

//go:embed *.html *.css *.js *.png *.svg *.ico *.webmanifest images/integrations/*.svg pierre/*.js.gz katex/*.js katex/*.css katex/*.woff2
var FS embed.FS
