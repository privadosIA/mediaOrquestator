package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"mediaOrquestator/internal/platform/settings"
	"mediaOrquestator/internal/promptfetch"
	"mediaOrquestator/internal/prompts"
	"mediaOrquestator/internal/server"
)

func hasSource(sources []promptfetch.Source, id string) bool {
	for _, s := range sources {
		if s.ID == id {
			return true
		}
	}
	return false
}

// runPrompts implements `orchestrator prompts fetch`: it fills the store of prompt presets from public datasets whose
// licence it reads from their cards (spec prompt-fetch).
func runPrompts(args []string, stdout, stderr io.Writer) int {
	return realDeps().prompts(args, stdout, stderr)
}

func (dp deps) prompts(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "fetch" {
		fmt.Fprintln(stderr, "Usage: orchestrator prompts fetch [--source id] [--max n] [--seed n] [--media-dir DIR] [--projects-dir DIR] [--db file] [--sources file] [--out file] [--dry-run]")
		return 2
	}
	fs := flag.NewFlagSet("prompts fetch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	source := fs.String("source", "", "Run only this source (a source that is off in the sources file runs when it is named)")
	maxRows := fs.Int("max", 0, "At most this many presets per source (default: each source's own limit)")
	seed := fs.Int("seed", 42, "Seed of the sample taken after cleaning: the same seed gives the same presets")
	mediaDir := fs.String("media-dir", "", "Media directory (or set MEDIA_DIR); the presets go to <media dir>/prompts")
	projectsDir := fs.String("projects-dir", "projects", "Directory of the projects: with no media directory the presets go to prompts/ beside it")
	sourcesFile := fs.String("sources", "", "A sources file to use instead of the one built into the program")
	dbPath := fs.String("db", defaultDB, "SQLite file of the database the presets are written to (used when no other database is saved in Settings)")
	out := fs.String("out", "", "Write the bundle (the file POST /api/prompts/import takes) here instead of writing the store")
	dry := fs.Bool("dry-run", false, "Download and report, but write nothing")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() > 0 || *maxRows < 0 {
		fmt.Fprintln(stderr, "fatal: unexpected arguments or a negative --max")
		return 2
	}
	sources := promptfetch.DefaultSources()
	if *sourcesFile != "" {
		var err error
		if sources, err = promptfetch.LoadSources(*sourcesFile); err != nil {
			fmt.Fprintf(stderr, "fatal: %v\n", err)
			return 1
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dir := prompts.StoreDir(*mediaDir, *projectsDir)
	var presets prompts.Store
	if *out == "" && !*dry && (*source == "" || hasSource(sources, *source)) { // an unknown source is the error of the run, whatever the database
		// the presets go to the database the server uses: the server lists them at once, with no restart
		st, err := openStore(*dbPath, *projectsDir, settings.GetDatabase(), server.WithMediaDir(*mediaDir))
		if err == nil {
			if err = st.CheckMigrated(); err != nil {
				_ = st.Close()
			} else {
				defer func() { _ = st.Close() }()
				presets = prompts.NewDB(st)
			}
		}
		if err != nil {
			fmt.Fprintf(stderr, "fatal: %v\n", err)
			return 1
		}
	}
	res, err := promptfetch.Run(ctx, promptfetch.Options{
		Sources: sources, Only: *source, Max: *maxRows, Seed: *seed, Dir: dir, Presets: presets, Out: *out, DryRun: *dry,
		Net: dp.promptsNet(), Log: func(m string) { fmt.Fprintln(stderr, "prompts fetch: "+m) },
	})
	if err != nil {
		fmt.Fprintf(stderr, "fatal: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, res.Report)
	switch {
	case *dry:
		fmt.Fprintf(stdout, "\ndry run: %d presets would be offered\n", len(res.Bundle.Items))
	case *out != "":
		fmt.Fprintf(stdout, "\nwrote %d presets to %s\n", len(res.Bundle.Items), *out)
	default:
		fmt.Fprintf(stdout, "\nwrote %d presets to the database (%d were already there)\n", res.Written, res.Already)
	}
	return 0
}
