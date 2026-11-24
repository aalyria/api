# bubblecast

bubblecast records a program drawing on a terminal as an
[asciicast](https://docs.asciinema.org/manual/asciicast/v2/), and renders the
recording as a GIF with [agg](https://github.com/asciinema/agg). The program
draws on a real pseudo-terminal of a fixed size, so a bubbletea TUI or a CLI's
colourful output is recorded exactly as a user would see it. Recording needs
Linux.

## Recording a urfave/cli app

Point the app's `Writer` and `ErrWriter` at the terminal `Record` hands you, and
run it as a user would:

```go
func record(ctx context.Context, app *cli.App) error {
	const term = "xterm-256color"
	// Make colour detection match a real terminal, whatever runs the demo.
	if err := bubblecast.SetTerminalEnv(term); err != nil {
		return err
	}
	rec := bubblecast.NewRecorder(bubblecast.Options{
		Cols: 100, Rows: 20,
		Title: "mytool list",
		Env:   map[string]string{"TERM": term},
	})
	rec.Print("$ mytool list\r\n")
	err := rec.Record(func(tty *os.File) error {
		app.Writer, app.ErrWriter = tty, tty
		return app.RunContext(ctx, []string{"mytool", "list"})
	})
	if err != nil {
		return err
	}
	rec.Print("$ ")
	// WorkspacePath writes next to the source when run with `bazel run`.
	return rec.WriteFile(bubblecast.WorkspacePath("docs/list.cast"))
}
```

A bubbletea program records the same way: pass the terminal to
`tea.WithInput` and `tea.WithOutput`.

## Rendering a GIF

`Render` runs agg, which must be installed, with settings that suit a README:

```go
err := bubblecast.Render(ctx, "docs/list.cast", "docs/list.gif", bubblecast.RenderOptions{})
```

or run agg yourself:

```sh
agg --font-size 14 --theme github-dark --last-frame-duration 6 --fps-cap 10 docs/list.cast docs/list.gif
```
