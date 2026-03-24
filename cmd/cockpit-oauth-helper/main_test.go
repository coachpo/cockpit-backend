package main

import (
	"flag"
	"testing"
)

func TestNewCommandFlagSet_ExposesTargetAndNoBrowserFlags(t *testing.T) {
	fs := newCommandFlagSet("cockpit-oauth-helper")
	if fs.Lookup("target") == nil {
		t.Fatal("expected -target flag to exist")
	}
	if fs.Lookup("no-browser") == nil {
		t.Fatal("expected -no-browser flag to exist")
	}
	flagNames := make([]string, 0)
	fs.VisitAll(func(f *flag.Flag) {
		flagNames = append(flagNames, f.Name)
	})
	if len(flagNames) != 2 {
		t.Fatalf("expected two command flags, got %v", flagNames)
	}
}

func TestParseCommandArgs_ParsesFlags(t *testing.T) {
	opts, err := parseCommandArgs("cockpit-oauth-helper", []string{"-target", "https://cockpit.example.com", "-no-browser"})
	if err != nil {
		t.Fatalf("parseCommandArgs() error = %v", err)
	}
	if opts.Target != "https://cockpit.example.com" {
		t.Fatalf("expected target to be propagated, got %q", opts.Target)
	}
	if !opts.NoBrowser {
		t.Fatal("expected -no-browser flag to set NoBrowser")
	}
}

func TestParseCommandArgs_RejectsPositionalArgs(t *testing.T) {
	if _, err := parseCommandArgs("cockpit-oauth-helper", []string{"https://cockpit.example.com"}); err == nil {
		t.Fatal("expected positional args to be rejected")
	}
}
