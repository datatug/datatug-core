package server

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/server/endpoints"
	"github.com/datatug/datatug/packages/storage"
	"github.com/datatug/datatug/packages/storage/filestore"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
	"time"
)

var agentHost string
var agentPort int

type httpServer struct {
	s *http.Server
}

func NewHttpServer() httpServer {
	return httpServer{}
}

func (s httpServer) Shutdown(ctx context.Context) error {
	return s.s.Shutdown(ctx)
}

// ServeHTTP starts HTTP server
func (s httpServer) ServeHTTP(pathsByID map[string]string, host string, port int) error {
	storage.NewDatatugStore = func(id string) (v storage.Store, err error) {
		//if v, err = filestore.NewStore(pathsByID); err != nil {
		//	err = fmt.Errorf("failed to create filestore for storage id=%v: %w", id, err)
		//	return
		//}
		panic("implement me")
		return
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

	router := httprouter.New()
	router.GlobalOPTIONS = http.HandlerFunc(globalOptionsHandler)
	router.HandlerFunc(http.MethodGet, "/", root)
	logWrapper := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/agent-info" {
				log.Println(r.Method, r.ContentLength, r.RequestURI)
			}
			handler(w, r)
		}
	}
	endpoints.RegisterDatatugHandlers("", router, endpoints.RegisterAllHandlers, logWrapper, func(r *http.Request) (context.Context, error) {
		return r.Context(), nil
	}, nil)

	s.s = &http.Server{
		Addr:           fmt.Sprintf("%v:%v", agentHost, agentPort),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 16,
		Handler:        router,
	}
	log.Printf("Serving on: http://%v:%v", agentHost, agentPort)

	return s.s.ListenAndServe()
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
`, filestore.GetProjectPath(storage.SingleProjectID))))
}
