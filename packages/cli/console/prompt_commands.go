package console

//
//import (
//	"github.com/datatug/datatug/packages/cli"
//	"github.com/c-bata/go-prompt"
//	"github.com/jessevdk/go-flags"
//	"golang.org/x/sys/windows" // replaced syscall
//	"log"
//	"strings"
//	"unsafe"
//)
//
//var currentPrompt *prompt.Prompt
//
//// NewCommandsPrompt creates new interactive prompt
//func NewCommandsPrompt() *prompt.Prompt {
//	currentPrompt = prompt.New(
//		promptExecutor,
//		completer,
//		prompt.OptionTitle("datatug"),
//		prompt.OptionPrefix("datatug>"),
//		//prompt.OptionInputTextColor(prompt.Yellow),
//	)
//	return currentPrompt
//}
//
//func promptExecutor(s string) {
//	log.Println("You've entered:", s)
//}
//
//func rootCommand(command *flags.Command) {
//	commands[command.Name] = command
//	for _, alias := range command.Aliases {
//		commands[alias] = command
//	}
//	rootCommands = append(rootCommands, prompt.Suggest{Text: command.Name, Description: command.ShortDescription})
//}
//
//func init() {
//	rootCommand(cli.ServeCommand)
//	rootCommand(cli.ScanDbCommand)
//	rootCommand(cli.ExecuteSQLCommand)
//	rootCommand(cli.ShowProjectCommand)
//}
//
//var commands = map[string]*flags.Command{}
//
//func completer(d prompt.Document) (suggests []prompt.Suggest) {
//	//textBeforeCursor := d.TextBeforeCursor()
//	//if textBeforeCursor == "" {
//	//	return rootCommands
//	//}
//	completionHandler := func(completions []flags.Completion) {
//		//log.Printf("completionHandler => items: %+v", items)
//		suggests = make([]prompt.Suggest, len(completions))
//		for i, completion := range completions {
//			if strings.HasPrefix(completion.Item, "/") {
//				if len(completion.Item) == 2 {
//					suggests[i].Text = strings.Replace(completion.Item, "/", "-", 1)
//				} else {
//					suggests[i].Text = strings.Replace(completion.Item, "/", "--", 1)
//				}
//			} else {
//				suggests[i].Text = completion.Item
//			}
//			suggests[i].Description = completion.Description
//		}
//	}
//	//commandValue := getCommandValue(d)
//	//if commandValue != "" {
//	//}
//
//	cli.Parser.CompletionHandler = completionHandler
//	cmd := d.TextBeforeCursor()
//	if cmd == "" {
//		cmd = " "
//	}
//	if args, err := syscallCommandLineToArgv(d.Text); err != nil {
//		log.Println(err)
//		return
//	} else if _, err = cli.Parser.ParseArgs(args); err != nil {
//		log.Println(err)
//		return
//	}
//
//	//wordBeforeCursorUntilSeparatorIgnoreNextToCursor := d.GetWordBeforeCursorUntilSeparatorIgnoreNextToCursor(" ")
//	//if commandValue == "" {
//	//	return prompt.FilterHasPrefix(rootCommands, wordBeforeCursor, true)
//	//}
//	//if commandValue != "" && wordBeforeCursor == "" && !strings.HasPrefix(wordBeforeCursorUntilSeparatorIgnoreNextToCursor, "-") ||
//	//	strings.HasPrefix(wordBeforeCursor, "-") {
//	//	return getCommandOptions(commandValue, wordBeforeCursor)
//	//}
//	return
//}
//
//// TODO: possibility to create package `prompt4flags` write an article
//// Taken from: os/os_windows_test.go
//// syscallCommandLineToArgv calls syscall.CommandLineToArgv
//// and converts returned result into []string.
//func syscallCommandLineToArgv(cmd string) ([]string, error) {
//	var utf16 []uint16
//	var err error
//	if utf16, err = windows.UTF16FromString(cmd); err != nil {
//		return nil, err
//	}
//	var argc int32
//	argv, err := windows.CommandLineToArgv(&utf16[0], &argc)
//	if err != nil {
//		return nil, err
//	}
//	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(argv))))
//
//	var args []string
//	for _, v := range (*argv)[:argc] {
//		args = append(args, windows.UTF16ToString((*v)[:]))
//	}
//	return args, nil
//}
//
//var rootCommands []prompt.Suggest
//
////func getCommandValue(d prompt.Document) string {
////	spaceIndex := strings.Index(d.Text, " ")
////	if spaceIndex > 0 {
////		return d.Text[0:spaceIndex]
////	}
////	return ""
////}
////
////func getCommandOptions(commandID, wordBeforeCursor string) (suggests []prompt.Suggest) {
////	cliCommand := commands[commandID]
////	if cliCommand == nil {
////		return
////	}
////	options := cliCommand.Options()
////	suggests = make([]prompt.Suggest, len(options))
////	for i, option := range options {
////		suggests[i] = prompt.Suggest{Text: "--" + option.LongName, Description: option.Description}
////	}
////	return prompt.FilterHasPrefix(suggests, wordBeforeCursor, true)
////}
