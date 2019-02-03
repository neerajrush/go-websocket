package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	_ "html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	_"strconv"
	"strings"
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

type  WebMsgOut struct {
	Msg_Type      string   `json:"msg_type"`
	Player_Name   string   `json:"new_player"`
	Draw_Number   int      `json:"draw_number"`
}

type WebMsg struct {
	MsgType int
	Msg []byte
}

var playersChan chan string
var adminChan chan *WebMsg

func GameLink(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity
	if err != nil {
		log.Println(err)
		return
	}

	go func () {
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println(err)
				return
			}
			fmt.Println("Msg:", string(msg))
			adminChan <- &WebMsg{ MsgType: msgType, Msg: msg, }
		}
	}()

	go func() {
		for {
			var msgType int
			var msg []byte
			var playerName string
			select {
				// Read message from browser
			case webMsg := <- adminChan:
				msgType = webMsg.MsgType
				msg = webMsg.Msg
				fmt.Println("Msg:", string(msg))
			case playerName = <- playersChan:
				fmt.Println("New Player is being added:", playerName)
				msg = []byte("new_player")
				msgType = 1 // TextMessage
			}
			if string(msg) == "gamelink" {
				// Print the message to the console
				fmt.Printf("%s is being sent: %s\n", conn.RemoteAddr(), gameLink)

				// Write message back to browser
				if err = conn.WriteMessage(msgType, []byte(gameLink)); err != nil {
					log.Println(err)
					return
				}
			}
			var webMsgOut WebMsgOut
			if string(msg) == "draw" {
				dNum := DrawNumber()
				w.Header().Set("Content-Type", "application/json")
				webMsgOut.Msg_Type = "draw_number"
				webMsgOut.Draw_Number =  dNum
				jsonNumber, err := json.Marshal(webMsgOut)
				if err != nil {
					fmt.Println(err)
					return	
				}

				// Print the message to the console
				fmt.Printf("%s is being sent: %d\n", conn.RemoteAddr(), dNum)

				// Write message back to browser
				if err = conn.WriteMessage(msgType, []byte(jsonNumber)); err != nil {
					log.Println(err)
					return
				}
			}
			if string(msg) == "new_player" {
				// Print the message to the console
				fmt.Printf("%s is being sent: %s\n", conn.RemoteAddr(), playerName)
				webMsgOut.Msg_Type = string(msg)
				webMsgOut.Player_Name = playerName
				jsonPlayer, err := json.Marshal(webMsgOut)
				if err != nil {
					fmt.Println(err)
					return	
				}
				w.Header().Set("Content-Type", "application/json")

				// Write message back to browser
				if err = conn.WriteMessage(msgType, []byte(jsonPlayer)); err != nil {
					log.Println(err)
					return
				}
			}
		}
	}()
}

type GameSheet struct {
	SheetId int
	Sheet   [][]int
}

var gamePlayers map[string]*GameSheet

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
			if !strings.Contains(string(msg), "update/") {
				fmt.Println("invalid request ..")
				return
			}
			sIndex := strings.Index(string(msg), "/")
			subMsg := string(msg)[sIndex+1:]
			pIndex := strings.Index(subMsg, "/")
			snId := subMsg[: pIndex]
			playerName := subMsg[pIndex+1:]
			fmt.Println("SessionId:", snId)
			fmt.Println("PlayerName:", playerName)
			if snId != sessionId {
				fmt.Println("Invalid Sessonid got:", snId, " expected:", sessionId)
				return
			}
			aSheet := PlayerSheet{Player_Sheet: getASheet()}
			//dNum := DrawNumber() + 20
			if err != nil {
				fmt.Println(err)
				return
			}
			w.Header().Set("Content-Type", "application/json")

			// Print the message to the console
			fmt.Printf("%s is being sent: %d\n", conn.RemoteAddr(), aSheet)

			jsonSheet, err := json.Marshal(aSheet)
			if err != nil {
				fmt.Println(err)
				return	
			}

			// Write message back to browser
			if err = conn.WriteMessage(msgType, []byte(jsonSheet)); err != nil {
				fmt.Println(err)
				return
			}
			if _,ok := gamePlayers[playerName]; !ok {
				gamePlayers[playerName] = &GameSheet{ SheetId: 1, Sheet: aSheet.Player_Sheet, }
			} else {
				gamePlayers[playerName].SheetId++
				gamePlayers[playerName].Sheet = aSheet.Player_Sheet
			}
			playersChan <- playerName
		}
	}()
}

type PlayerSheet struct {
	Player_Sheet [][]int `json:"player_sheet"`
}

func readFile(title string) ([]byte, error) {
	filename := "html/" + title + ".html"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func init() {
	gamePlayers = make(map[string]*GameSheet)
	playersChan = make(chan string, 1)
	adminChan = make(chan *WebMsg)
}

func main() {
	router := NewRouter()
	log.Fatal(http.ListenAndServe(":8081", router))
}

func getASheet() [][]int {
	cols := make([][]int, 5)
	for i, _ := range cols {
		cols[i] = make([]int, 5)
		for j, _ := range cols[i] {
			cols[i][j] = rand.Intn(100)
		}
	}
	return cols
}
