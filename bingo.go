/*
*
* Package: bingo 
* It defines all structures for building a sesssion for a team.
*
*/ 
package  main

import  (
	"log"
	"fmt"
	"sync"
	"math/rand"
	"sort"
	"time"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"net/http"
	"strings"
)

type StatusResp struct {
	Status bool `json:"status"`
}

type PongResp struct {
	Msg_Type string   `json:"msg_type"`
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

var routes = Routes {
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

//
// defined bingo sheet dimentions 5X5
//
const (
	SHEET_DIM = 5
)

//
// Defined a sheet with colxrow
//
type BingoSheet struct {
	SheetId      int
	Sheet [][]int
	Conn         *websocket.Conn
	totalMatchNeeded int
	drawMatchCount  int
	oneColMatch  bool
	oneRowMatch  bool
	oneDiagonalMatch  bool
	fullHouseMatch  bool
}

type BingoGame struct {
	GameId string
	GameLink string
	GamePlayers        map[string]*BingoSheet
        draws []int
	drawCount int
	winnerOneCol  bool
	winnerOneRow  bool
	winnerOneDiagonal  bool
	winnerFullHouse  bool

}

type BingoSessions struct {
	activeSessions  map[string]*BingoGame
}

var  games *BingoSessions
var  gamesLock sync.Mutex

func NewBingoGame(gameId string) (*BingoGame, error) {
	bGame := BingoGame{ GameId: gameId,
			    GameLink: "http://192.168.11.23/players/" + gameId,
	                    GamePlayers: make(map[string]*BingoSheet),
			    draws: make([]int, 100), 
		            drawCount: 0,
			    winnerOneCol: false,
			    winnerOneRow: false,
			    winnerOneDiagonal: false,
			    winnerFullHouse: false, }
	return &bGame,  nil
}

func NewBingoSheet() (*BingoSheet, error) {
	bingoSheet := BingoSheet{ SheetId: 1, Sheet: make([][]int, SHEET_DIM), }
	for i, _ := range bingoSheet.Sheet {
		rows := make([]int, SHEET_DIM)
		bingoSheet.Sheet[i] = rows
	}
	return &bingoSheet, nil
}

func  FindBingoSession(gameId string) (*BingoGame, error) {
	if gameId == "" {
		return nil, fmt.Errorf("couldn't find bingo session for nil gameId.")
	}

	if b, ok := games.activeSessions[gameId]; ok {
		return  b, nil
	}

	return nil, fmt.Errorf("couldn't find bingo sesssion, probably session for gameId is not active %v", gameId)
}

func (b *BingoGame) AddPlayer(player string) (*BingoSheet, error) {
	if player == "" {
		return nil, fmt.Errorf("couldn't add the nil player", player)
	}

	gamesLock.Lock()
	defer gamesLock.Unlock()

	aSheet, _ := NewBingoSheet()
	aSheet.populateSheet()

	b.GamePlayers[player] = aSheet

	log.Printf("%v: added new player", b.GameId, player)

	return aSheet, nil
}

func (s *BingoSheet) populateSheet() {
	for i, col := range s.Sheet {
		for  j,_ := range col {
			s.Sheet[i][j] = uniqRandNumber(col, i)
			s.totalMatchNeeded += 1
		}
		sort.Ints(s.Sheet[i])
		if  i ==  2 {
			// Wildcard the center location
			s.Sheet[i][2] = -1
			s.totalMatchNeeded -= 1
		} else {
			// Wildcard the random location
			genIn <- 5
			r := <- genOut
			if r != 0 {
				s.Sheet[i][r] = -1
				s.totalMatchNeeded -= 1
			}
		}
	}
}

func (s *BingoSheet) findMatch(draw int) bool {
	for i, col := range s.Sheet {
		for  j,_ := range col {
			if s.Sheet[i][j] == draw {
				s.drawMatchCount += 1
			}
		}
	}
	log.Println("DrawMatchCount:", s.drawMatchCount, " total Match Needed:", s.totalMatchNeeded)
	if s.drawMatchCount == s.totalMatchNeeded {
		s.fullHouseMatch = true
		return true
	}
	return false
}

func uniqRandNumber(aCol []int, idx int) int {
	min := idx * 15
	max := (idx+1) * 15
	for {
		genIn <- max - min
		r := <- genOut + min
		if r == 0  {
			continue
		}
		duplicate := false
		for _, v := range aCol {
			if v == r {
				duplicate = true
				break
			}
		}
		if  duplicate {
			continue
		}
		return r
	}
}


func DrawUniqRandNumber(draws []int) int {
	dCount := 0
	for {
		if dCount >= 75 {
			break
		}
		genIn <- 75
		r := <- genOut
		if r == 0  {
			continue
		}
		duplicate := false
		for _, v := range draws {
			if v == r {
				duplicate = true
				break
			}
		}
		if  duplicate {
			dCount += 1
			continue
		}
		return r
	}
	return 0
}

var gotWinner chan string

func (b *BingoGame) Play(dChan chan int) {
	for b.drawCount = 0; b.drawCount < 75; b.drawCount++ {
		b.draws[b.drawCount] = DrawUniqRandNumber(b.draws)
		dChan <- b.draws[b.drawCount]
		for player := range b.GamePlayers {
			if b.GamePlayers[player].findMatch(b.draws[b.drawCount])  {
				gotWinner <- player
				close(gotWinner)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func Home(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	w.Header().Set("Content-Type", "text/html")
	body, _ := readFile("index")
	w.Write(body)
}

func Players(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API: ", r.URL.Path)
	vars := mux.Vars(r)
	sessionId := vars["sessId"]
	fmt.Println("SessionId:", sessionId)
	if _,ok := games.activeSessions[sessionId]; !ok {
		fmt.Println("No active session:", sessionId)
		http.NotFound(w, r)
		return
	}
	fmt.Println("found active session:", sessionId)
	w.Header().Set("Content-Type", "text/html")
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
	Match         bool     `json:"match"`
	Col           int      `json:"col"`
	Row           int      `json:"row"`
	Winner        bool     `json:"winner"`
}

type DrawnNumRec struct {
	DrawnNum int
	Match    bool
	Col      int
	Row      int
	Conn	 *websocket.Conn
	WinnerName string
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
	fmt.Println("websocket upgraded..")

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
		var adminConn *websocket.Conn
		var playerName string
		var sessionId string
		for {
			select {
				// Read message from browser
			case webMsgIn := <- adminWebInChan:
				msgType = webMsgIn.MsgType
				msg = webMsgIn.Msg
				adminConn = webMsgIn.Conn
				if string(msg) == "ping" {
					pongResp := PongResp{ Msg_Type: "pong", }
					jsonPong, err := json.Marshal(pongResp)
					w.Header().Set("Content-Type", "application/json")
					fmt.Println("GameLink => Reply: pong")
					if err = adminConn.WriteMessage(msgType, []byte(jsonPong)); err != nil {
						log.Println(err)
						return
					}
					continue
				}
				fmt.Println("GameLink => WebMsgIn:", string(msg))
				sIndex := strings.Index(string(msg), "/")
				status := string(msg)[0:sIndex];
				sessionId = string(msg)[sIndex+1:]
				log.Println("cmd:", status)
				log.Println("SessionId:", sessionId)
				if status == "status" {
					if _, ok := games.activeSessions[sessionId]; !ok {
					    gameLink := "http://192.168.11.23/players/" + sessionId
					    //gameLink := "http://71.202.98.110/players/" + sessionId
					    games.activeSessions[sessionId],_ = NewBingoGame(sessionId)
					    games.activeSessions[sessionId].GameLink = gameLink
					    log.Println("New session created:", sessionId)
				        }
				} else {
					if _, ok := games.activeSessions[sessionId]; !ok {
						log.Println("No session found:", sessionId)
						return
					}
					log.Println("Draw a number for the session:", sessionId)
					msg = []byte("drawnumber")
				}
			case playerName = <- players2AdminChan:
				fmt.Println("Admin: Received meaasge ==> New Player is being added:", playerName)
				msg = []byte("new_player")
				msgType = 1 // TextMessage
			}
			var webMsgOut WebMsgOut
			if string(msg) == "gamelink" && len(games.activeSessions) > 0 {
				var gameLink string
				for sId, v := range games.activeSessions {
					gameLink = v.GameLink
					sessionId = sId
				}
				// Print the message to the console
				fmt.Printf("%s is being sent: %s\n", adminConn.RemoteAddr(), gameLink)

				// Write message back to browser
				if err = adminConn.WriteMessage(msgType, []byte(gameLink)); err != nil {
					log.Println(err)
					return
				}
			} else if string(msg) == "new_player"  && len(games.activeSessions) > 0 {
				// Print the message to the console
				fmt.Printf("Admin: update for new player %s is being sent: %s\n", conn.RemoteAddr(), playerName)
				webMsgOut.Msg_Type = string(msg)
				webMsgOut.Player_Name = playerName
				webMsgOut.Winner = false
				jsonPlayer, err := json.Marshal(webMsgOut)
				if err != nil {
					fmt.Println(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")

				// Write message back to browser
				if err = adminConn.WriteMessage(msgType, []byte(jsonPlayer)); err != nil {
					log.Println(err)
					return
				}
			} else if string(msg) == "drawnumber"  && len(games.activeSessions) > 0 {
				bingoSession := games.activeSessions[sessionId]
				dNum := DrawUniqRandNumber(bingoSession.draws)
				if dNum == 0 {
					log.Println("DrawNumber ==> 0")
				} else {
					bingoSession.draws[bingoSession.drawCount] = dNum
					bingoSession.drawCount += 1
				}
				if bingoSession.drawCount == 75 {
					log.Println("DrawNumber's list is full. We should already have a winner.")
					sort.Ints(bingoSession.draws)
					log.Println(bingoSession.draws)
				}
				w.Header().Set("Content-Type", "application/json")
				webMsgOut.Msg_Type = "draw_number"
				webMsgOut.Draw_Number =  dNum
				jsonNumber, err := json.Marshal(webMsgOut)
				if err != nil {
					fmt.Println(err)
					return
				}

				// Print the message to the console
				fmt.Printf("%s is being sent: %d\n", adminConn.RemoteAddr(), dNum)

				// Write message back to browser
				if err = adminConn.WriteMessage(msgType, []byte(jsonNumber)); err != nil {
					log.Println(err)
					return
				}
				winnerFound := false
				winnerName  := ""
				for player,playerSheet := range bingoSession.GamePlayers {
					log.Printf("sending drawn number: %d ==> player: %s Addr: %s\n", dNum, player, playerSheet.Conn.RemoteAddr())
					match := false
					col := 0
					row := 0
					for  j, xCol := range playerSheet.Sheet {
						for i, rVal := range xCol {
							if rVal == dNum {
								match = true
								col = j
								row = i
								break
							}
							if match {
								break
							}
						}
					}
					if match {
					    log.Printf("match found: %d ==> player: %s, col: %d row: %d\n", dNum, player, col, row)
					    if playerSheet.findMatch(dNum)  {
						        winnerFound = true
							winnerName = player
							fmt.Printf("Admin: found winner: %s and is being sent: %s\n", conn.RemoteAddr(), player)
							webMsgOut.Msg_Type = "winner"
							webMsgOut.Player_Name = player
							webMsgOut.Winner = true
							jsonPlayer, err := json.Marshal(webMsgOut)
							if err != nil {
								fmt.Println(err)
								return
							}
							w.Header().Set("Content-Type", "application/json")

							// Write message back to browser
							if err = adminConn.WriteMessage(msgType, []byte(jsonPlayer)); err != nil {
								log.Println(err)
								return
							}
					    }
					}
					drawnNumChan <- &DrawnNumRec{ DrawnNum: dNum,
								      Match: match,
								      Col: col,
								      Row: row,
								      Conn: playerSheet.Conn,
							              WinnerName: winnerName, }
				}
				if winnerFound {
					log.Println("GAME OVER ==> WINNER:", winnerName)
					log.Println("Killing the session", sessionId)
				        delete(games.activeSessions, sessionId)
				}
			}
		}
	}()
}

var playerWebInChan chan *WebMsgIn
var drawnNumChan chan *DrawnNumRec

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
			if string(msg) == "ping" {
				playerWebInChan <- &WebMsgIn{ MsgType: msgType, Msg: msg, Conn: conn, }
			} else {
				if !strings.Contains(string(msg), "add/") {
					fmt.Println("invalid request ..")
					return
				}
				playerWebInChan <- &WebMsgIn{ MsgType: msgType, Msg: msg, Conn: conn, }
			}
		}
	}()

	go func() {
		msgType := 1
		var snId, playerName string
		var webMsgOut WebMsgOut
		for {
			select {
			case webMsgIn := <- playerWebInChan:
				if string(webMsgIn.Msg) == "ping" {
					playerConn := webMsgIn.Conn
					pongResp := PongResp{ Msg_Type: "pong", }
					jsonPong, err := json.Marshal(pongResp)
					w.Header().Set("Content-Type", "application/json")
					if err = playerConn.WriteMessage(msgType, []byte(jsonPong)); err != nil {
						log.Println(err)
						return
					}
					continue
				}
				sIndex := strings.Index(string(webMsgIn.Msg), "/")
				subMsg := string(webMsgIn.Msg)[sIndex+1:]
				pIndex := strings.Index(subMsg, "/")
				snId = subMsg[: pIndex]
				playerName = subMsg[pIndex+1:]
				fmt.Println("WebMsgIn SessionId:", snId)
				fmt.Println("WebMsgIn PlayerName:", playerName)
				if _, ok := games.activeSessions[snId]; !ok {
					fmt.Println("Invalid SessonId got:", snId)
					return
				}
				// Adding new player
				playerConn := webMsgIn.Conn
				if _, ok := games.activeSessions[snId].GamePlayers[playerName]; !ok {
					games.activeSessions[snId].GamePlayers[playerName],_ = NewBingoSheet()
					games.activeSessions[snId].GamePlayers[playerName].Conn = playerConn
					games.activeSessions[snId].GamePlayers[playerName].populateSheet()
				} else {  // update the existing players sheet.
					games.activeSessions[snId].GamePlayers[playerName].SheetId++
					games.activeSessions[snId].GamePlayers[playerName].Sheet = getASheet()
					games.activeSessions[snId].GamePlayers[playerName].populateSheet()
				}
				webMsgOut.Msg_Type = "player_sheet"
				webMsgOut.Player_Sheet = games.activeSessions[snId].GamePlayers[playerName].Sheet
				fmt.Printf("Reply to: %s is being sent: %d\n", playerConn.RemoteAddr(), webMsgOut.Player_Sheet)
				jsonObj, err := json.Marshal(webMsgOut)
				if err != nil {
					fmt.Println(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")

				// Write message back to browser
				if err = playerConn.WriteMessage(msgType, []byte(jsonObj)); err != nil {
					fmt.Println(err)
					return
				}
				players2AdminChan <- playerName
			case drawnNumRec := <- drawnNumChan:
					webMsgOut.Msg_Type = "draw_number"
					webMsgOut.Draw_Number = drawnNumRec.DrawnNum
					webMsgOut.Match = drawnNumRec.Match
					webMsgOut.Col = drawnNumRec.Col
					webMsgOut.Row = drawnNumRec.Row
					playerConn := drawnNumRec.Conn
					if drawnNumRec.WinnerName == "" {
						webMsgOut.Winner = false
					} else {
						webMsgOut.Winner = true
						webMsgOut.Player_Name = drawnNumRec.WinnerName
					}
					fmt.Printf("%s is being sent: %d\n", playerConn.RemoteAddr(), webMsgOut.Draw_Number)

					jsonObj, err := json.Marshal(webMsgOut)
					if err != nil {
						fmt.Println(err)
						return
					}
					w.Header().Set("Content-Type", "application/json")

					// Write message back to browser
					if err = playerConn.WriteMessage(msgType, []byte(jsonObj)); err != nil {
						fmt.Println(err)
						return
					}
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

func getASheet() [][]int {
	bSheet, err := NewBingoSheet()
	if err != nil {
		log.Panic(err)
	}

	return  bSheet.Sheet
}

func init() {

	games = &BingoSessions{activeSessions: make(map[string]*BingoGame), }

	adminWebInChan = make(chan *WebMsgIn)
	players2AdminChan = make(chan string, 1)

	playerWebInChan = make(chan *WebMsgIn)
	drawnNumChan = make(chan *DrawnNumRec)

	gotWinner = make(chan string, 1)
	genIn = make(chan int)
	genOut = make(chan int)
}

func main() {
	go generateRandomNumber()

	router := NewRouter()
	log.Fatal(http.ListenAndServe("192.168.11.23:80", router))
}

func checkForWinner(bGame *BingoGame) {

	dChan := make (chan int, 100)
	var winner string

	for  {
		select {
		case dNum := <- dChan:
			fmt.Print(dNum, " ")
		case player := <- gotWinner:
			fmt.Println("Got Winner: ", player)
			winner = player
			break
		}
		if winner != "" {
			break
		}
	}

	genIn <- -1

	if ok := TestWinner(bGame, winner); ok {
		log.Println("Test PASS... winner is",  winner)
		return
	}
	log.Println("Test FAIL... winner is",  winner)
}

var genIn, genOut chan int

func generateRandomNumber() {
	runGenRand := func(c chan int, id int) {
		rand.Seed(time.Now().Unix() + int64(id*9999999))
		for g := range  genIn {
			if g == -1 {
				c <- g
				return 
			}
			c <- rand.Intn(g)
		}
	}
	c1 := make(chan int)
	go runGenRand(c1, 1) 
	c2 := make(chan int)
	go runGenRand(c2, 2) 
	c3 := make(chan int)
	go runGenRand(c3, 3) 
	var x int 
	for {
		select {
			case x = <- c1:
			case x = <- c2:
			case x = <- c3:
		}
		if x == -1 {
			return
		}
		genOut <- x
	}
}

func matchesIn(draws []int, val int) bool {
	for _, v := range draws {
		if v == val {
			fmt.Print(val, " ")
			return true
		}
	}
	fmt.Println("")
	return false
}

func TestWinner(b *BingoGame, winner string) bool {
	winningSheet := b.GamePlayers[winner]
	for _, col := range winningSheet.Sheet {
		for _, val := range col {
			if val == -1 {
				continue
			}
			if  !matchesIn(b.draws, val) {
				return false
			}
		}
	}

	fmt.Println("Start checking for all other players....")
	allWinners := make([]string, 0)

	for player,bSheet := range b.GamePlayers {
		fmt.Println("Lets look for player: ", player)
		matchFound := true
		for _, col := range bSheet.Sheet {
			for _, val := range col {
				if val == -1 {
					continue
				}
				if  !matchesIn(b.draws, val) {
					matchFound = false
					break
				}
			}
			if !matchFound {
				break
			}
		}
		if !matchFound {
			fmt.Println("No full match for player: ", player)
		} else {
			fmt.Println("Found winner: ", player)
			allWinners = append(allWinners, player)
		}
	}
	fmt.Println("All winners:", allWinners)
	return true
}
