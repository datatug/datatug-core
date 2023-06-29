package commands

import (
	"errors"
	"fmt"
	config2 "github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server"
	"github.com/datatug/datatug/packages/storage/filestore"
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
	Host      string `short:"h" long:"host" default:"localhost"`
	Port      int    `short:"o" long:"port" default:"8989"`
	Local     bool   `long:"local" description:"opens UI on default localhost:4200"`
	ClientURL string `long:"client-url" description:"Default is https://datatug.app/pwa/agent/localhost:8989"`
}

// Execute executes serve command
func (v *serveCommand) Execute(_ []string) (err error) {
	var config config2.Settings
	config, err = config2.GetSettings()
	if err != nil {
		return err
	}

	pathsByID := make(map[string]string)
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
		pathsByID = getProjPathsByID(config)
	}

	serverConfig := config.Server

	if v.Host == "" {
		v.Host = serverConfig.Host
	}
	if v.Port == 0 {
		v.Port = serverConfig.Port
	}
	if v.ClientURL == "" {
		if v.Local {
			//goland:noinspection HttpUrlsUsage
			v.ClientURL = fmt.Sprintf("http://%s:%d", v.Host, v.Port) // consider choosing some unique default port
		} else {
			v.ClientURL = fmt.Sprintf("https://datatug.app/pwa/repo/%s:%d", v.Host, v.Port)
		}
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
	httpServer := server.NewHttpServer()
	// TODO: implement graceful shutdown
	return httpServer.ServeHTTP(pathsByID, v.Host, v.Port)
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
