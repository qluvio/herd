package cmd

import (
	"fmt"
	"io"
	"path"
	"time"

	"github.com/seveas/herd/scripting"
	"github.com/seveas/readline"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var interactiveCmd = &cobra.Command{
	Use:   "interactive [glob [filters] [<+|-> glob [filters]...]]",
	Short: "Interactive shell for running commands on a set of hosts",
	Long: `With Herd's interactive shell, you can easily run multiple commands, and
manipulate the host list between commands. You can even use the result of
previous commands as filters.`,
	RunE:                  runInteractive,
	DisableFlagsInUseLine: true,
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}

func runInteractive(cmd *cobra.Command, args []string) error {
	splitAt := cmd.ArgsLenAtDash()
	if splitAt != -1 {
		return fmt.Errorf("Command provided, but interactive mode doesn't support that")
	}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	engine, err := setupScriptEngine()
	if err != nil {
		return err
	}
	if err = engine.ParseCommandLine(args, splitAt); err != nil {
		logrus.Error(err.Error())
		return err
	}
	fn := path.Join(viper.GetString("HistoryDir"), time.Now().Format("2006-01-02T15:04:05.json"))
	engine.Execute()

	// Enter interactive mode
	il := &interactiveLoop{engine: engine}
	il.run()
	engine.End()
	return engine.SaveHistory(fn)
}

type interactiveLoop struct {
	engine *scripting.ScriptEngine
}

func (l *interactiveLoop) run() {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          l.prompt(),
		AutoComplete:    l.autoComplete(),
		HistoryFile:     path.Join(viper.GetString("HistoryDir"), "interactive"),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		logrus.Errorf("Unable to start interactive mode: %s", err)
		return
	}
	defer rl.Close()
	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			continue
		} else if err == io.EOF {
			break
		} else if err != nil {
			logrus.Error(err.Error())
			break
		}
		if line == "exit" {
			break
		}
		if err := l.engine.ParseCodeLine(line + "\n"); err != nil {
			logrus.Error(err.Error())
			continue
		}
		l.engine.Execute()
		rl.SetPrompt(l.prompt())
	}
}

func (l *interactiveLoop) prompt() string {
	return fmt.Sprintf("herd [%d hosts] $ ", len(l.engine.ActiveHosts()))
}

func (l *interactiveLoop) autoComplete() readline.AutoCompleter {
	p := readline.PcItem
	return readline.NewPrefixCompleter(
		p("set",
			p("Timeout"),
			p("HostTimeout"),
			p("ConnectTimeout"),
			p("Parallel"),
			p("Output"),
			p("LogLevel"),
		),
		p("add hosts"),
		p("remove hosts"),
		p("list hosts",
			p("oneline"),
		),
		p("run"),
	)
}
