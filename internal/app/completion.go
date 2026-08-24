package app

import (
	"fmt"
	"strings"
)

func (a *Application) cmdCompletion(opts extendedOptions, positional []string) int {
	shell := "bash"
	if len(positional) > 0 {
		shell = strings.ToLower(positional[0])
	}
	commands := "doctor config alias tag profile bind unbind recommend statusline watch history stats forecast backup metrics completion account target run"
	var script string
	switch shell {
	case "bash":
		script = fmt.Sprintf(`_agy_swap_complete() { COMPREPLY=($(compgen -W '%s add list limits logout next switch remove status update version limit' -- "${COMP_WORDS[COMP_CWORD]}")); }; complete -F _agy_swap_complete agy-swap`, commands)
	case "zsh":
		script = fmt.Sprintf("#compdef agy-swap\n_arguments '1:command:(%s add list limits logout next switch remove status update version limit)'", commands)
	case "fish":
		script = fmt.Sprintf("complete -c agy-swap -f -a '%s add list limits logout next switch remove status update version limit'", commands)
	case "powershell", "pwsh":
		script = `Register-ArgumentCompleter -Native -CommandName agy-swap -ScriptBlock { param($wordToComplete, $commandAst, $cursorPosition); 'doctor','config','alias','tag','profile','bind','recommend','statusline','watch','history','stats','forecast','backup','metrics','completion','account','target','run','add','list','limits','logout','next','switch','remove','status','update','version','limit' | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) } }`
	default:
		return a.extendedError("completion", opts, fmt.Errorf("unsupported shell %q", shell))
	}
	if opts.JSON {
		return a.extendedResult("completion", opts, map[string]string{"shell": shell, "script": script}, nil)
	}
	fmt.Fprintln(a.Out, script)
	return 0
}
