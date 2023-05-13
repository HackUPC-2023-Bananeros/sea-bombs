package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	lock sync.RWMutex
)

const (
	Create  Type = 3
	Move    Type = 4
	End     Type = 5
	Check   Type = 6
	Started Type = 7
	Update  Type = 8
)

type Type int
type Direction int
type EndGame struct {
	Event   Type     `json:"event"`
	Players []string `json:"players"`
}
type MoveRequest struct {
	Event Type
	AxisX int
	AxisY int
}
type Request struct {
	Event    Type     `json:"type"`
	Players  []string `json:"players"`
	Player   string   `json:"player"`
	Movement int      `json:"button_pressed"`
}
type Participant struct {
	Game  uuid.UUID
	Role  Direction
	Addr  net.UDPAddr
	Ready bool
	End   bool
}
type Game struct {
	Players       []string
	ActiveButtons []int
}

func main() {
	games := make(map[uuid.UUID]Game)
	participants := make(map[string]Participant)
	serverPort := 7000
	server := start_server(serverPort)
	server_addr := net.UDPAddr{}
	go gameStatusSender(server, games, participants)

	for {
		data, addr := readMessage(server)
		fmt.Println(data)
		switch data.Event {
		case Create:
			createGame(data, games, participants)
			server_addr = addr
			break
		case Check:
			checkUser(data, addr, games, participants, server)
			break
		case Move:
			updateMoveVector(data, games, participants)
		case End:
			endGame(server, server_addr, data, games, participants)
		}

	}

}
func readMessage(server *net.UDPConn) (Request, net.UDPAddr) {
	p := make([]byte, 1024)
	nn, addr, err := server.ReadFromUDP(p)
	if err != nil {
		fmt.Printf("Read err  %v", err)
	}
	msg := p[:nn]
	data := Request{}
	error := json.Unmarshal(msg, &data)
	if error != nil {
		fmt.Printf("JSON decode err %v", err)
	}
	return data, *addr
}
func start_server(serverPort int) *net.UDPConn {
	if len(os.Args) > 1 {
		if v, err := strconv.Atoi(os.Args[1]); err != nil {
			fmt.Printf("Invalid port %v, err %v", os.Args[1], err)
			os.Exit(-1)
		} else {
			serverPort = v
		}
	}

	addr := net.UDPAddr{
		Port: serverPort,
		IP:   net.ParseIP("0.0.0.0"),
	}
	server, err := net.ListenUDP("udp", &addr)
	if err != nil {
		fmt.Printf("Listen err %v\n", err)
		os.Exit(-1)
	}
	return server
}
func allUsersEnd(game uuid.UUID, games map[uuid.UUID]Game, participants map[string]Participant) bool {
	for players := range games[game].Players {
		if !participants[games[game].Players[players]].End {
			return false
		}
	}
	return true
}
func updateUserFinish(participant *Participant) Participant {
	participant.End = true
	return *participant
}
func endGame(server *net.UDPConn, server_addr net.UDPAddr, request Request, games map[uuid.UUID]Game, participants map[string]Participant) {
	participant := participants[request.Player]
	participants[request.Player] = updateUserFinish(&participant)
	game := participants[request.Player].Game
	if allUsersEnd(game, games, participants) {
		data := &EndGame{End, games[game].Players}
		b, _ := json.Marshal(data)

		go func(conn *net.UDPConn, raddr *net.UDPAddr, msg []byte) {
			_, err := conn.WriteToUDP([]byte(fmt.Sprintf("%s", msg)), raddr)
			if err != nil {
				fmt.Printf("Response err %v", err)
			}
		}(server, &server_addr, b)

		for user := range games[game].Players {
			delete(participants, games[game].Players[user])
		}
		delete(games, participant.Game)
	}
}
func gameStatusSender(server *net.UDPConn, games map[uuid.UUID]Game, participants map[string]Participant) {
	for {
		for id, game := range games {
			lock.Lock()

			if allUsersReady(id, games, participants) {
				request := &MoveRequest{Update,
					game.ActiveButtons[2] - game.ActiveButtons[3],
					game.ActiveButtons[0] - game.ActiveButtons[1]}

				b, _ := json.Marshal(request)

				for _, participant := range game.Players {
					address := participants[participant].Addr

					server.WriteToUDP([]byte(fmt.Sprintf("%s", b)), &address)

				}

			}
			lock.Unlock()

		}
		time.Sleep(100 * time.Millisecond)

	}
}
func updateMoveVector(request Request, games map[uuid.UUID]Game, participants map[string]Participant) {
	game := games[participants[request.Player].Game]
	participant := participants[request.Player]
	if &participant.Game != nil && &game.Players != nil && len(game.Players) > 0 {
		lock.Lock()
		game.ActiveButtons[participant.Role] = request.Movement
		lock.Unlock()
	}
}
func checkUser(request Request, address net.UDPAddr, games map[uuid.UUID]Game, participants map[string]Participant, server *net.UDPConn) {
	participant := participants[request.Player]
	participants[request.Player] = updateUserAddr(&participant, address)
	if allUsersReady(participants[request.Player].Game, games, participants) {
		communicateStart(games[participants[request.Player].Game].Players, participants, server)
	}
}
func communicateStart(members []string, participants map[string]Participant, server *net.UDPConn) {
	req := &Request{Event: Started}
	b, err := json.Marshal(req)
	if err != nil {
		fmt.Printf("Error: %s", err)
		return
	}

	for val := range members {
		claddr := participants[members[val]].Addr
		go func(conn *net.UDPConn, raddr *net.UDPAddr, msg []byte) {
			_, err := conn.WriteToUDP([]byte(fmt.Sprintf("%s", msg)), raddr)
			if err != nil {
				fmt.Printf("Response err %v", err)
			}
		}(server, &claddr, b)
	}
}
func allUsersReady(game uuid.UUID, games map[uuid.UUID]Game, participants map[string]Participant) bool {
	for players := range games[game].Players {
		if !participants[games[game].Players[players]].Ready {
			return false
		}
	}
	return true
}
func updateUserAddr(participant *Participant, address net.UDPAddr) Participant {
	participant.Addr = address
	participant.Ready = true
	return *participant
}
func createGame(request Request, games map[uuid.UUID]Game, participants map[string]Participant) {
	gameUuid := uuid.New()
	games[gameUuid] = Game{[]string{}, make([]int, 4)}
	game := games[gameUuid]
	for index, user := range request.Players {
		games[gameUuid] = *addPlayers(&game, user)
		participants[request.Players[index]] = Participant{gameUuid, Direction(index), net.UDPAddr{}, false, false}
	}
}
func addPlayers(game *Game, user string) *Game {
	game.Players = append(game.Players, user)
	return game
}
