package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

const sample = `# Notification preview

**Author:** @octocat · **Review:** approved

## Review summary

Review **Fix slow uploads** in [acme/app](https://github.com/acme/app).
Keep the change *small and focused*. Use ` + "`context.Context`" + ` for cancellation.

> This change keeps uploads responsive on slow connections.

### Before merge

- [x] Add request cancellation
- [ ] Confirm the timeout behavior

| Check | Result |
| --- | --- |
| Tests | Passed |
| Lint | Passed |

` + "```go\n// Keep cancellation explicit.\nfunc upload(ctx context.Context) error {\n    const attempts = 3\n    log.Print(\"upload started\")\n    return send(ctx)\n}\n```\n"

func main() {
	light := flag.Bool("light", false, "show the light theme")
	export := flag.String("export", "", "write both Glamour themes to this directory")
	flag.Parse()
	if err := run(*light, *export); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(light bool, dir string) error {
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		for _, name := range []string{"dark", "light"} {
			data, err := json.MarshalIndent(theme.MarkdownStyle(name == "light"), "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "github-"+name+".json"), append(data, '\n'), 0644); err != nil {
				return err
			}
		}
		return nil
	}
	out, err := theme.Render(sample, "", 70, light, false)
	if err != nil {
		return err
	}
	_, err = fmt.Print(out)
	return err
}
