package httpserv

import (
	"log"
	"net/http"
	"uploader/queue"
)

// this service should watch the new tasks in DB and process them

type Server struct {
	mux   *http.ServeMux
	port  string
	queue *queue.Queue
}

func (s *Server) init() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.getRoot)
	mux.HandleFunc("/favicon.ico", s.get404)
	mux.HandleFunc("/convert", s.convert)
	mux.HandleFunc("/probe", s.probe)
	mux.HandleFunc("/check", s.check)

	s.mux = mux
}

func (s *Server) Run() {
	log.Println("Start http server on :" + s.port)
	if err := http.ListenAndServe(":"+s.port, s.mux); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) getRoot(w http.ResponseWriter, r *http.Request) {
	log.Println("get " + r.RequestURI)
}

func (s *Server) get404(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func NewServer(port string, queue *queue.Queue) (*Server, error) {
	s := &Server{
		port:  port,
		queue: queue,
	}
	s.init()
	return s, nil
}
