package webassets

import "embed"

//go:embed index.html app.js devices.js styles.css
var Files embed.FS
