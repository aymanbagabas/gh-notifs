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

Review **Fix slow uploads** in [acme/app](https://github.com/acme/app).

> This change keeps uploads responsive on slow connections.

- [x] Add request cancellation
- [ ] Confirm the timeout behavior

` + "```go\nfunc upload(ctx context.Context) error {\n    return send(ctx)\n}\n```\n"

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
