package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
 	"html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"
	"strings"
)

type Todo struct {
	GroupName string
	SecretPhrase  string
}

type TodoPageData struct {
	PageTitle string
	GameLink  string
	Todos     []Todo
}

//var tmpl *template.Template

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
		"Todos",
		"GET",
		"/todos",
		Todos,
	},
	Route{
		"Status",
		"GET",
		"/status",
		Status,
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
		"GameLink",
		"GET",
		"/gamelink",
		GameLink,
	},
	Route{
		"PlayersDraw",
		"GET",
		"/playersdraw",
		PlayersDraw,
	},
}

type GameSheet struct {
	SheetId      int
	Sheet        [][]int
	WebInChan    chan *WebMsgIn
	DrawChan     chan int
}

type GameSession struct {
	GameId             string
	GameSessionLink    string
	GamePlayers        map[string]*GameSheet
}

// map sessionId ==> gameLink
var gameSessions map[string]*GameSession

func Home(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	body, _ := readFile("index")
	w.Write(body)
}

func Admin(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	if !strings.Contains(r.URL.Path, "admin/") {
		fmt.Println("invalid request ..")
		return
	}
	aIndex := strings.Index(r.URL.Path, "/")
	subMsg := string(r.URL.Path)[aIndex+1:]
	sIndex := strings.Index(subMsg, "/")
	sessionId := subMsg[sIndex+1:]
	log.Println("SessionId:", sessionId)
	gameLink := "http://localhost:8081/players/" + sessionId
	if _, ok := gameSessions[sessionId]; !ok {
		gameSessions[sessionId] = &GameSession{GameId: sessionId, GameSessionLink: gameLink, GamePlayers: make(map[string]*GameSheet), }
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
}

func Players(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	vars := mux.Vars(r)
	sessionId := vars["sessId"]
	fmt.Println("SessionId:", sessionId)
	if _,ok := gameSessions[sessionId]; !ok {
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
	rand.Seed(time.Now().Unix())
	return  rand.Intn(100)
}

// WebSocket In&Out structs.
type WebMsgIn struct {
	MsgType int
	Msg []byte
	Conn  *websocket.Conn
}

type  WebMsgOut struct {
	Msg_Type      string   `json:"msg_type"`
	Player_Name   string   `json:"new_player"`
	Draw_Number   int      `json:"draw_number"`
	Player_Sheet  [][]int  `json:"player_sheet"`
}

// Admin reads websocket messages from admin client.
var adminWebInChan chan *WebMsgIn

// Admin reads players name from each players client.
// Each players sends player names to admin.
var players2AdminChan chan string

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
			fmt.Println("GameLink => Msg:", string(msg))
			adminWebInChan <- &WebMsgIn { MsgType: msgType, Msg: msg, Conn: conn, }
		}
	}()

	go func() {
		var msgType int
		var msg []byte
		var rConn *websocket.Conn
		var playerName string
		var sessionId string
		for {
			select {
				// Read message from browser
			case webMsgIn := <- adminWebInChan:
				msgType = webMsgIn.MsgType
				msg = webMsgIn.Msg
				rConn = webMsgIn.Conn
				fmt.Println("GameLink => WebMsgIn:", string(msg))
				sIndex := strings.Index(string(msg), "/")
				status := string(msg)[0:sIndex];
				sessionId = string(msg)[sIndex+1:]
				log.Println("cmd:", status)
				log.Println("SessionId:", sessionId)
				if status == "status" {
					if _, ok := gameSessions[sessionId]; !ok {
					    gameLink := "http://localhost:8081/players/" + sessionId
					    gameSessions[sessionId] = &GameSession{GameId: sessionId,
								       GameSessionLink: gameLink,
								       GamePlayers: make(map[string]*GameSheet),
								    }
					    log.Println("New session created:", sessionId)
				        }
				} else {
					if _, ok := gameSessions[sessionId]; !ok {
					    	log.Println("No session found:", sessionId)
						return
					}
					msg = []byte("drawnumber")
				}
			case playerName = <- players2AdminChan:
				fmt.Println("Admin: Received meaasge ==> New Player is being added:", playerName)
				msg = []byte("new_player")
				msgType = 1 // TextMessage
			}
			if string(msg) == "gamelink" && len(gameSessions) > 0 {
				var gameLink string
				for sId, v := range gameSessions {
					gameLink = v.GameSessionLink
					sessionId = sId
				}
				// Print the message to the console
				fmt.Printf("%s is being sent: %s\n", rConn.RemoteAddr(), gameLink)

				// Write message back to browser
				if err = rConn.WriteMessage(msgType, []byte(gameLink)); err != nil {
					log.Println(err)
					return
				}
			}
			var webMsgOut WebMsgOut
			if string(msg) == "drawnumber"  && len(gameSessions) > 0 {
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
				fmt.Printf("%s is being sent: %d\n", rConn.RemoteAddr(), dNum)

				// Write message back to browser
				if err = rConn.WriteMessage(msgType, []byte(jsonNumber)); err != nil {
					log.Println(err)
					return
				}
				games := gameSessions[sessionId]
				for player,playerSheet := range games.GamePlayers {
					log.Println("found player: ", player, "sending drawn number: ", dNum)
					playerSheet.DrawChan <- dNum
				}
			}
			if string(msg) == "new_player"  && len(gameSessions) > 0 {
				// Print the message to the console
				fmt.Printf("Admin: update for new player %s is being sent: %s\n", conn.RemoteAddr(), playerName)
				webMsgOut.Msg_Type = string(msg)
				webMsgOut.Player_Name = playerName
				jsonPlayer, err := json.Marshal(webMsgOut)
				if err != nil {
					fmt.Println(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")

				// Write message back to browser
				if err = rConn.WriteMessage(msgType, []byte(jsonPlayer)); err != nil {
					log.Println(err)
					return
				}
			}
		}
	}()
}

var webInChan chan *WebMsgIn

func PlayersDraw(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("Upgrading to websocket for ", r.URL.Path, " for remote client: ", conn.RemoteAddr())

	go func () {
		for {
			log.Println("Reading request from websocket:", conn.RemoteAddr())
			// Read message from browser
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println(err)
				return
			}
			log.Println("PlayersDraw Msg:", string(msg))
			if !strings.Contains(string(msg), "add/") {
				fmt.Println("invalid request ..")
				return
			}
			webInChan <- &WebMsgIn{ MsgType: msgType, Msg: msg, Conn: conn, }
		}
	}()

	go func() {
		msgType := 1
		var snId, playerName string
		var webMsgOut WebMsgOut
		for {
			select {
			case webMsgIn := <- webInChan:
				rConn := webMsgIn.Conn
				sIndex := strings.Index(string(webMsgIn.Msg), "/")
				subMsg := string(webMsgIn.Msg)[sIndex+1:]
				pIndex := strings.Index(subMsg, "/")
				snId = subMsg[: pIndex]
				playerName = subMsg[pIndex+1:]
				fmt.Println("WebMsgIn SessionId:", snId)
				fmt.Println("WebMsgIn PlayerName:", playerName)
				if _, ok := gameSessions[snId]; !ok {
					fmt.Println("Invalid SessonId got:", snId)
					return
				}
				// Adding new player
				if _, ok := gameSessions[snId].GamePlayers[playerName]; !ok {
					gameSessions[snId].GamePlayers[playerName] = &GameSheet{ SheetId: 1,
										      Sheet: getASheet(),
										      WebInChan: make(chan *WebMsgIn),
										      DrawChan: make(chan int),
									   }
				} else {  // update the existing players sheet.
					gameSessions[snId].GamePlayers[playerName].SheetId++
					gameSessions[snId].GamePlayers[playerName].Sheet = getASheet() 
				}
				webMsgOut.Msg_Type = "player_sheet"
				webMsgOut.Player_Sheet = gameSessions[snId].GamePlayers[playerName].Sheet
				fmt.Printf("Reply to: %s is being sent: %d\n", rConn.RemoteAddr(), webMsgOut.Player_Sheet)
				jsonObj, err := json.Marshal(webMsgOut)
				if err != nil {
					fmt.Println(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")

				// Write message back to browser
				if err = rConn.WriteMessage(msgType, []byte(jsonObj)); err != nil {
					fmt.Println(err)
					return
				}
				players2AdminChan <- playerName
				/*
			case drawNumber := <- gameSessions[snId].GamePlayers[playerName].DrawChan:
					webMsgOut.Msg_Type = "draw_number"
					webMsgOut.Draw_Number = drawNumber
					fmt.Printf("%s is being sent: %d\n", conn.RemoteAddr(), webMsgOut.Draw_Number)

					jsonObj, err := json.Marshal(webMsgOut)
					if err != nil {
						fmt.Println(err)
						return
					}
					w.Header().Set("Content-Type", "application/json")

					// Write message back to browser
					if err = conn.WriteMessage(msgType, []byte(jsonObj)); err != nil {
						fmt.Println(err)
						return
					}
			*/
			}
		}
	}()
}

func Todos(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("html/todos.html"))
	data := TodoPageData{
		PageTitle: "Let's play Bingo!",
		GameLink: "http://localhost:8081/players/sessionId",
		Todos: []Todo{
			{GroupName: "ABC", SecretPhrase: "Test-1"},
		},
	}
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, data)
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

	gameSessions = make(map[string]*GameSession)

	adminWebInChan = make(chan *WebMsgIn)
	players2AdminChan = make(chan string, 1)

	webInChan = make(chan *WebMsgIn)
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
