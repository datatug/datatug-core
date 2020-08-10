package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/server"
	"log"
	"os"
	"os/exec"
	runtime "runtime"
	"strings"
)

// ServeCommand executes serve command
//var ServeCommand *flags.Command

func init() {
	var err error
	_, err = Parser.AddCommand("serve",
		"Serves HTTP server to provide API for UI",
		"Serves HTTP server to provide API for UI. Default port is 8989",
		&serveCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// serveCommand defines parameters for serve command
type serveCommand struct {
	ProjectDir string `short:"t" long:"directory" description:"Project directory"`
	Host       string `short:"h" long:"host"`
	Port       int    `short:"o" long:"port" default:"8989"`
	Dev        bool   `short:"d" long:"dev"`
	UIUrl      string `long:"uiurl"`
}

// Execute executes serve command
func (v *serveCommand) Execute(_ []string) (err error) {
	var projPaths []string
	if v.ProjectDir != "" {
		projPaths = strings.Split(v.ProjectDir, ",")
	} else {
		var config ConfigFile
		if config, err = getConfig(); err != nil {
			return err
		}
		if err = printConfig(config, os.Stdout); err != nil {
			return err
		}
		projPaths = make([]string, len(config.Projects))
		for i, p := range config.Projects {
			projPaths[i] = p.Path
		}
	}

	if v.Host == "" {
		v.Host = "localhost"
	}

	if v.UIUrl == "" {
		v.UIUrl = "http://localhost:8100"
	}

	if len(projPaths) == 1 {
		openBrowser(fmt.Sprintf("%v/project/.@%v:%v", v.UIUrl, v.Host, v.Port))
	} else {
		openBrowser(fmt.Sprintf("%v/agent/%v:%v", v.UIUrl, v.Host, v.Port))
	}

	return server.ServeHTTP(projPaths, v.Host, v.Port)
}

func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		//goland:noinspection SpellCheckingInspection
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}
}
