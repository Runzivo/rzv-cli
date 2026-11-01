package cli

import (
	"strconv"
	"strings"
)

// command is one parsed invocation: the command name, its positional
// arguments and its --options. Parsing follows the 0.8.0 CLI: "--name value",
// "--name=value", boolean flags, and "--" to end option parsing.
type command struct {
	name    string
	args    []string
	options map[string]string
	flags   map[string]bool
	// profile is the -p / --profile choice. It is read before any command
	// runs, so commands never list it.
	profile    string
	hasProfile bool
}

// shortOptions are the one-letter spellings of --profile and --workspace.
var shortOptions = map[string]string{"-p": "profile", "-w": "workspace"}

// booleanOptions never consume the next argument as a value.
var booleanOptions = map[string]bool{
	"json":                    true,
	"help":                    true,
	"no-browser":              true,
	"version":                 true,
	"allow-insecure-loopback": true,
	"remove-logo":             true,
	"personal":                true,
	"open":                    true,
	"all":                     true,
	"stdio":                   true,
	"write":                   true,
	"refresh":                 true,
}

func parse(argv []string) (*command, error) {
	cmd := &command{options: map[string]string{}, flags: map[string]bool{}}
	var positional []string
	seen := map[string]bool{}
	optionsEnded := false
	for i := 0; i < len(argv); i++ {
		value := argv[i]
		if long, ok := shortOptions[value]; ok && !optionsEnded {
			value = "--" + long
		}
		if optionsEnded || !strings.HasPrefix(value, "--") {
			if !optionsEnded && (value == "-h") {
				seen["help"] = true
				cmd.flags["help"] = true
				continue
			}
			positional = append(positional, value)
			continue
		}
		if value == "--" {
			optionsEnded = true
			continue
		}
		option := value[2:]
		name, assigned, hasValue := strings.Cut(option, "=")
		if name == "" {
			return nil, usageError("Invalid option %q.", value)
		}
		if seen[name] {
			return nil, usageError("--%s was provided more than once.", name)
		}
		seen[name] = true
		if booleanOptions[name] {
			if hasValue {
				return nil, usageError("--%s does not take a value.", name)
			}
			cmd.flags[name] = true
			continue
		}
		if hasValue {
			cmd.options[name] = assigned
			continue
		}
		if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "--") {
			return nil, usageError("--%s requires a value.", name)
		}
		cmd.options[name] = argv[i+1]
		i++
	}
	if len(positional) > 0 {
		cmd.name = positional[0]
		cmd.args = positional[1:]
	}
	if profile, ok := cmd.options["profile"]; ok {
		cmd.profile, cmd.hasProfile = profile, true
		delete(cmd.options, "profile")
	}
	return cmd, nil
}

// allow rejects any option or flag not in the allowed list.
func (c *command) allow(names ...string) error {
	allowed := map[string]bool{"json": true, "help": true}
	for _, n := range names {
		allowed[n] = true
	}
	for name := range c.options {
		if !allowed[name] {
			return usageError("--%s is not valid for this command.", name)
		}
	}
	for name := range c.flags {
		if !allowed[name] {
			return usageError("--%s is not valid for this command.", name)
		}
	}
	return nil
}

// option returns a non-blank option value, or "" when it is absent.
func (c *command) option(name string) (string, error) {
	value, ok := c.options[name]
	if !ok {
		return "", nil
	}
	if strings.TrimSpace(value) == "" {
		return "", usageError("--%s requires a value.", name)
	}
	return value, nil
}

func (c *command) required(name string) (string, error) {
	value, err := c.option(name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", usageError("--%s is required.", name)
	}
	return value, nil
}

// boolOption returns an explicitly supplied true or false option.
func (c *command) boolOption(name string) (bool, error) {
	value, err := c.required(name)
	if err != nil {
		return false, err
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil || (value != "true" && value != "false") {
		return false, usageError("--%s must be true or false.", name)
	}
	return parsed, nil
}

// expectArgs checks the number of positional arguments after the command name.
func (c *command) expectArgs(n int, usage string) error {
	if len(c.args) != n {
		return usageError("Use `%s`.", usage)
	}
	for _, a := range c.args {
		if a == "" {
			return usageError("Use `%s`.", usage)
		}
	}
	return nil
}

func (c *command) json() bool { return c.flags["json"] }
