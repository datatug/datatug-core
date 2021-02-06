package commands

import (
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server"
	"github.com/datatug/datatug/packages/store/filestore"
	"log"
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
	projectBaseCommand
	Host      string `short:"h" long:"host"`
	Port      int    `short:"o" long:"port" default:"8989"`
	Dev       bool   `long:"dev"`
	ClientURL string `long:"client-url"`
}

// Execute executes serve command
func (v *serveCommand) Execute(_ []string) (err error) {
	var pathsByID map[string]string
	if v.ProjectDir != "" {
		if strings.Contains(v.ProjectDir, ";") {
			return errors.New("serving multiple specified throw a command line argument is not supported yet")
		}
		var projectFile models.ProjectFile
		if projectFile, err = filestore.LoadProjectFile(v.ProjectDir); err != nil {
			return fmt.Errorf("failed to load project file: %w", err)
		}
		pathsByID[projectFile.ID] = v.ProjectDir
	} else {
		var config ConfigFile
		config, err = getConfig()
		if err != nil {
			return err
		}
		pathsByID = getProjPathsByID(config)
	}

	if v.Host == "" {
		v.Host = "localhost"
	}

	if v.ClientURL == "" {
		v.ClientURL = "http://localhost:8100"
	}

	var agent string
	if v.Port == 0 || v.Port == 80 {
		agent = v.Host
	} else {
		agent = fmt.Sprintf("%v:%v", v.Host, v.Port)
	}

	url := v.ClientURL + "/agent/" + agent

	if err := openBrowser(url); err != nil {
		_, _ = fmt.Printf("failed to open browser with URl=%v: %v", url, err)
	}
	return server.ServeHTTP(pathsByID, v.Host, v.Port)
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		//goland:noinspection SpellCheckingInspection
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return fmt.Errorf("unsupported platform")

	}
}
