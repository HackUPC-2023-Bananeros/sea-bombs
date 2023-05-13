package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net"
	"os"
	"strconv"
)

const (
	PressButton   MoveAction = 1
	ReleaseButton MoveAction = 0

	Up    Direction = 0
	Down  Direction = 1
	Right Direction = 2
	Left  Direction = 3

	Create  Type = 3
	Move    Type = 4
	End     Type = 5
	Check   Type = 6
	Started Type = 7
)

type MoveAction int
type Type int
type Direction int
type MoveRequest struct {
	AxisX int8
	AxisY int8
}
type Request struct {
	Event    Type       `json:"event"`
	Players  []string   `json:"players"`
	Player   string     `json:"player"`
	Movement MoveAction `json:"button_pressed"`
}
type Participant struct {
	Game  uuid.UUID
	Role  Direction
	Addr  net.UDPAddr
	Ready bool
}

func main() {
	games := make(map[uuid.UUID][]string)
	participants := make(map[string]Participant)

	serverPort := 7000
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
	fmt.Printf("Listen at %v\n", addr.String())

	for {
		p := make([]byte, 1024)
		nn, raddr, err := server.ReadFromUDP(p)
		if err != nil {
			fmt.Printf("Read err  %v", err)
			continue
		}

		msg := p[:nn]
		data := Request{}
		error := json.Unmarshal(msg, &data)
		if error != nil {
			fmt.Printf("JSON decode err %v", err)
			continue
		}
		switch data.Event {
		case Create:
			create_game(data, games, participants)
			break
		case Check:
			check_user(data, *raddr, games, participants, server)
			break
		case Move:

		}
	}

}
func check_user(request Request, address net.UDPAddr, games map[uuid.UUID][]string, participants map[string]Participant, server *net.UDPConn) {
	participant := participants[request.Player]
	participants[request.Player] = update_user_addr(&participant, address)
	if all_users_ready(participants[request.Player], games, participants) {
		communicate_start(games[participants[request.Player].Game], participants, server)
	}
}
func communicate_start(members []string, participants map[string]Participant, server *net.UDPConn) {
	req := &Request{Event: Started}
	b, err := json.Marshal(req)
	if err != nil {
		fmt.Printf("Error: %s", err)
		return
	}

	for val := range members {
		claddr := participants[members[val]].Addr
		go func(conn *net.UDPConn, raddr *net.UDPAddr, msg []byte) {
			_, err := conn.WriteToUDP([]byte(fmt.Sprintf("Pong: %s", msg)), raddr)
			if err != nil {
				fmt.Printf("Response err %v", err)
			}
		}(server, &claddr, b)
	}
}
func all_users_ready(participant Participant, games map[uuid.UUID][]string, participants map[string]Participant) bool {
	for players := range games[participant.Game] {
		if !participants[games[participant.Game][players]].Ready {
			return false
		}
	}
	return true
}
func update_user_addr(participant *Participant, address net.UDPAddr) Participant {
	participant.Addr = address
	participant.Ready = true
	return *participant
}
func create_game(request Request, games map[uuid.UUID][]string, participants map[string]Participant) {
	gameUuid := uuid.New()
	games[gameUuid] = []string{}
	for user := range request.Players {
		games[gameUuid] = append(games[gameUuid], request.Players[user])
		participants[request.Players[user]] = Participant{gameUuid, Direction(user), net.UDPAddr{}, false}
	}
}
