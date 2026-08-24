package app

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

func (a *Application) cmdTarget(opts extendedOptions, positional []string) int {
	sub := "list"
	if len(positional) > 0 {
		sub = positional[0]
	}
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("target", opts, err)
	}
	switch sub {
	case "list":
		data := make([]map[string]any, 0, len(settings.Targets)+4)
		for name, target := range settings.Targets {
			_, lookErr := exec.LookPath(target.Command)
			data = append(data, map[string]any{"name": name, "command": target.Command, "enabled": target.Enabled, "available": lookErr == nil})
		}
		for _, builtin := range []string{"agy", "gemini", "claude", "gpt"} {
			if _, exists := settings.Targets[builtin]; exists {
				continue
			}
			_, lookErr := exec.LookPath(builtin)
			data = append(data, map[string]any{"name": builtin, "command": builtin, "enabled": false, "available": lookErr == nil, "experimental": true})
		}
		sort.Slice(data, func(i, j int) bool { return fmt.Sprint(data[i]["name"]) < fmt.Sprint(data[j]["name"]) })
		if opts.JSON {
			return a.extendedResult("target list", opts, data, nil)
		}
		for _, item := range data {
			fmt.Fprintf(a.Out, "%s  %s  available=%v enabled=%v\n", item["name"], item["command"], item["available"], item["enabled"])
		}
		return 0
	case "check":
		if len(positional) < 2 {
			return a.extendedError("target check", opts, errors.New("usage: target check NAME"))
		}
		name := positional[1]
		target, ok := settings.Targets[name]
		if !ok {
			target = TargetConfig{Command: name}
		}
		path, lookErr := exec.LookPath(target.Command)
		data := map[string]any{"name": name, "command": target.Command, "available": lookErr == nil, "path": path}
		if opts.JSON {
			return a.extendedResult("target check", opts, data, nil)
		}
		if lookErr != nil {
			return a.extendedError("target check", opts, lookErr)
		}
		fmt.Fprintf(a.Out, "%s is available at %s\n", name, path)
		return 0
	case "set":
		if len(positional) < 3 {
			return a.extendedError("target set", opts, errors.New("usage: target set NAME COMMAND"))
		}
		name, command := cleanText(positional[1]), strings.TrimSpace(positional[2])
		if err := validateAliasName(name); err != nil {
			return a.extendedError("target set", opts, err)
		}
		if command == "" || strings.ContainsAny(command, "\t\r\n") {
			return a.extendedError("target set", opts, errors.New("target command must be one executable path or name"))
		}
		settings.Targets[name] = TargetConfig{Command: command, Enabled: true}
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("target set", opts, err)
		}
		if opts.JSON {
			return a.extendedResult("target set", opts, map[string]any{"name": name, "target": settings.Targets[name]}, nil)
		}
		fmt.Fprintf(a.Out, "%s -> %s\n", name, command)
		return 0
	case "remove", "rm":
		if len(positional) < 2 {
			return a.extendedError("target remove", opts, errors.New("usage: target remove NAME"))
		}
		delete(settings.Targets, positional[1])
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("target remove", opts, err)
		}
		return 0
	default:
		return a.extendedError("target", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}
