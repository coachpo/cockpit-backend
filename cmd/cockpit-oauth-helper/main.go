package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	internalcmd "github.com/coachpo/cockpit-backend/internal/cmd"
	"github.com/coachpo/cockpit-backend/internal/logging"
	log "github.com/sirupsen/logrus"
)

const (
	targetUsage    = "Cockpit backend base URL that should receive forwarded OAuth callbacks"
	noBrowserUsage = "Print the OAuth URL without trying to open a browser automatically"
)

func init() {
	logging.SetupBaseLogger()
}

type commandOptions struct {
	Target    string
	NoBrowser bool
}

func newCommandFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.String("target", "", targetUsage)
	fs.Bool("no-browser", false, noBrowserUsage)
	fs.Usage = func() {
		out := fs.Output()
		_, _ = fmt.Fprintf(out, "Usage of %s\n", name)
		fs.VisitAll(func(f *flag.Flag) {
			s := fmt.Sprintf("  -%s", f.Name)
			usageName, unquoteUsage := flag.UnquoteUsage(f)
			if usageName != "" {
				s += " " + usageName
			}
			if len(s) <= 4 {
				s += "\t"
			} else {
				s += "\n    "
			}
			s += unquoteUsage
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
				s += fmt.Sprintf(" (default %s)", f.DefValue)
			}
			_, _ = fmt.Fprint(out, s+"\n")
		})
	}
	return fs
}

func parseCommandArgs(name string, args []string) (commandOptions, error) {
	fs := newCommandFlagSet(name)
	if err := fs.Parse(args); err != nil {
		return commandOptions{}, err
	}
	if fs.NArg() > 0 {
		return commandOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return commandOptions{
		Target:    strings.TrimSpace(fs.Lookup("target").Value.String()),
		NoBrowser: fs.Lookup("no-browser").Value.String() == "true",
	}, nil
}

func main() {
	opts, err := parseCommandArgs(os.Args[0], os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Errorf("failed to parse command line flags: %v", err)
		os.Exit(2)
	}
	if err := internalcmd.RunOAuthHelper(context.Background(), internalcmd.OAuthHelperOptions{
		Target:    opts.Target,
		NoBrowser: opts.NoBrowser,
	}); err != nil {
		log.Errorf("oauth helper failed: %v", err)
		os.Exit(1)
	}
}
