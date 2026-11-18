# Grunt: The JavaScript Task Runner

[![Built with Grunt](https://gruntjs.com/builtwith.svg)](https://gruntjs.com/)
[![Tested with QUnit](https://qunitjs.com/testedwith.svg)](https://qunitjs.com/)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bhttps%3A%2F%2Fgithub.com%2Fgruntjs%2Fgrunt.svg?type=shield)](https://app.fossa.io/projects/git%2Bhttps%3A%2F%2Fgithub.com%2Fgruntjs%2Fgrunt?ref=badge_shield)

<img align="right" height="260" src="https://gruntjs.com/img/grunt-logo-no-wordmark.svg">

### Documentation

Visit the [gruntjs.com](https://gruntjs.com/) website for all the things.

### Go

This repository is grunt 1.6.3 for gruntfiles written in Go: the module
`github.com/gruntjs/grunt-go` (Go 1.27, standard library only). A gruntfile is a Go
program that hands its body to `grunt.Main`, and behaves like `grunt` run by grunt-cli
1.5.0 next to a `Gruntfile.js` with the same body:

```go
package main

import (
	"github.com/gruntjs/grunt-go"
	"github.com/gruntjs/grunt-go/js"
)

func main() {
	grunt.Main(func(g *grunt.Grunt) {
		g.InitConfig(grunt.O("pkg", grunt.O("name", "demo")))
		g.RegisterTask("default", "Say hello.", grunt.TaskFunc(func(t *grunt.Task) any {
			g.Log.Writeln("Hello from " + js.ToString(g.Config.Get("pkg.name")))
			return nil
		}))
	})
}
```

`go run . build:dist --verbose` takes the same options and task arguments as `grunt`.
The API is in the package documentation (`go doc github.com/gruntjs/grunt-go`). Template
code runs on the JavaScript subset in [docs/template-js.md](docs/template-js.md), with the
helpers in [docs/template-helpers.md](docs/template-helpers.md). What differs from the
JavaScript version is listed in [MIGRATION.md](MIGRATION.md).

Run the tests with `go test ./...`.

### Support

We support the latest version with security and bug fixes. The previous versions are all end-of-life and will not receive any security or bug fixes.

Our OpenJS Ecosystem Sustainability Program partner [HeroDevs](https://www.herodevs.com/support/grunt-nes) provides drop-in replacements for older versions of Grunt that are kept up-to-date for security and compliance issues. Learn More.

### Version Support

| Version | Supported? | Commercial Support                                                      |
| ------- | ---------- | ----------------------------------------------------------------------- |
| 1.6     | YES        |                                                                         |
| 1.5     | NO         | [Available Here](https://www.herodevs.com/support/grunt-nes) |
| 1.4     | NO         | [Available Here](https://www.herodevs.com/support/grunt-nes) |
| 1.3     | NO         | [Available Here](https://www.herodevs.com/support/grunt-nes) |
| 1.2     | NO         | [Available Here](https://www.herodevs.com/support/grunt-nes) |
| 1.1     | NO         | [Available Here](https://www.herodevs.com/support/grunt-nes) |
| 1.0     | NO         | [Available Here](https://www.herodevs.com/support/grunt-nes) |
| 0.4     | NO         | [Available Here](https://www.herodevs.com/support/grunt-nes) |

### Contributing

Before you make an issue, please read our [Contributing](https://gruntjs.com/contributing) guide.

### Release History

See the [CHANGELOG.md](./CHANGELOG.md).

### License

[MIT](LICENSE)
