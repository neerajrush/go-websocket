package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"log"
	"net/http"
	_ "html/template"
)

type GameSession struct {
	HostName     string
	SecretPhrase string
}

type GamePage struct {
	Title string
	Page  []byte
}

type StatusResp struct {
	Status bool `json:"status"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route

func NewRouter() *mux.Router {
	router := mux.NewRouter().StrictSlash(false)
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
		"/",
		Home,
	},
	Route{
		"Players",
		"GET",
		"/players/{sessId}",
		Players,
	},
	Route{
		"PlayersDraw",
		"GET",
		"/playersdraw",
		PlayersDraw,
	},
	Route{
		"Admin",
		"POST",
		"/admin",
		Admin,
	},
	Route{
		"GameLink",
		"GET",
		"/gamelink",
		GameLink,
	},
}

var sessionId string
var gameLink string

func Home(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	body, _ := readFile("index")
	w.Write(body)
}

func NewSession(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	vars := mux.Vars(r)
	groupName := vars["groupname"]
	secretPhrase := vars["secretphrase"]
	sessionId = groupName + "-" + secretPhrase
	fmt.Println("SessionId:", sessionId)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	body, _ := readFile("index")
	w.Write(body)
}

func Admin(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	err := r.ParseForm()
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	sessionId = r.Form.Get("groupname") + "-" + r.Form.Get("secretphrase")
	gameLink = "http://localhost:8081/players/" + sessionId
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	body, _ := readFile("admin")
	w.Write(body)
}

func Players(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	vars := mux.Vars(r)
	sessId := vars["sessId"]
	fmt.Println("SessId:", sessId)
	if sessId != sessionId {
		http.NotFound(w, r)
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	body, _ := readFile("players")
	w.Write(body)
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

func DrawNumber() int {
	return 23
}

func GameLink(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity
	if err != nil {
		log.Println(err)
		return
	}

	go func() {
		for {
			// Read message from browser
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println(err)
				return
			}
			fmt.Println("Msg:", string(msg))
			if string(msg) == "gamelink" {
				// Print the message to the console
				fmt.Printf("%s is being sent: %s\n", conn.RemoteAddr(), gameLink)

				// Write message back to browser
				if err = conn.WriteMessage(msgType, []byte(gameLink)); err != nil {
					log.Println(err)
					return
				}
			}
			if string(msg) == "draw" {
				dNum := DrawNumber()

				// Print the message to the console
				fmt.Printf("%s is being sent: %d\n", conn.RemoteAddr(), dNum)

				// Write message back to browser
				if err = conn.WriteMessage(msgType, []byte(strconv.Itoa(dNum))); err != nil {
					log.Println(err)
					return
				}
			}
		}
	}()
}

func PlayersDraw(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity
	if err != nil {
		log.Println(err)
		return
	}

	go func() {
		for {
			// Read message from browser
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			fmt.Println("Msg:", string(msg))
			if string(msg) != "update" {
				return
			}
			dNum := DrawNumber() + 20
			if err != nil {
				return
			}

			// Print the message to the console
			fmt.Printf("%s is being sent: %d\n", conn.RemoteAddr(), dNum)

			// Write message back to browser
			if err = conn.WriteMessage(msgType, []byte(strconv.Itoa(dNum))); err != nil {
				return
			}
		}
	}()
}

func readFile(title string) ([]byte, error) {
	filename := "html/" + title + ".html"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func main() {
	router := NewRouter()
	log.Fatal(http.ListenAndServe(":8081", router))
}
