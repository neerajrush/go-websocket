package main

import (
	"log"
	"net/http"
	"ioutil"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader {
		ReadBufferSize: 1024
		WriteBufferSize: 1024
}

type Route struct {
		Name         string
		Method       string
		Pattern      string
		HandlerFunc  http.HandlerFunc
}

type Routes []Route

func NewRouter() *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
			router.Methods(route.Method).Path(route.Pattern).Name(route.Name).Handler(route.HandlerFunc)
	}

	return router
}

var routes = Routes{
	Route{
		"Status",
		"GET",
		"/status",
		Status,
	},
	Route{
		"NewSession",
		"POST",
		"/new",
		NewSession,
	},
	Route{
		"Home",
		"GET",
		"/[index|home]",
		Home,
	},
	Route{
		"Players",
		"GET",
		"/players?sessionId=",
		Players,
	},
}

func Home(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	w.Header().Set("Content-Type", "html/text")
	w.WriteHeader(http.StatusOK)
	w.Write(readFile("index"))
}

func Status(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	statusResp := StatusResp{
		Status: true,
	}
	if err := json.NewEncoder(w).Encode(statusResp); err != nil {
		panic(err)
	}
}

func readFile(title string) ([]byte, error) {
	filename := "html/" + title  + ".html"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func main() {
	router := NewRouter()
	err := http.ListenAndServe(":8081", router)
	panic(err)
}
