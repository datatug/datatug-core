package server

import (
	"fmt"
	"github.com/datatug/datatug/packages/store"
	"github.com/datatug/datatug/packages/store/filestore"
	"log"
	"net/http"
	"time"
)

var agentHost string
var agentPort int

func init() {
	initRouter()
}

// ServeHTTP starts HTTP server
func ServeHTTP(pathsByID map[string]string, host string, port int) error {
	var err error

	if store.Current, err = filestore.NewStore(pathsByID); err != nil {
		return err
	}

	if host == "" {
		agentHost = "localhost"
	} else {
		agentHost = host
	}

	if port == 0 {
		agentPort = 8989
	} else {
		agentPort = port
	}

	s := http.Server{
		Addr:           fmt.Sprintf("%v:%v", agentHost, agentPort),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 16,
		Handler:        router,
	}
	log.Printf("Serving on: http://%v:%v", agentHost, agentPort)

	return s.ListenAndServe()
}

func root(writer http.ResponseWriter, _ *http.Request) {
	_, _ = writer.Write([]byte(fmt.Sprintf(`
<html>
<head>
	<title>DataTug Agent</title>
	<style>body{font-family: Verdana}</style> 
</head>
<body>
	<h1>DataTug API</h1>
	<hr>
	Serving project from %v
	<hr>

	<h2>API endpoints</h2>
	<ul>
		<li><a href=/project>Project</a></li>
	</ul>

	<h2>Test endpoints</h2>
	<ul>
		<li><a href=/ping>Ping (pong) - simply returns a "pong" string</a></li>
		<li>
			<a href=/projects>/projects</a> - list of projects hosted by this agent
		</li>
	</ul>

<footer>
	&copy; 2020 <a href=https://datatug.app target=_blank>DataTug.app</a>
</footer>
</body>
</html>
`, filestore.GetProjectPath(store.SingleProjectID))))
}
